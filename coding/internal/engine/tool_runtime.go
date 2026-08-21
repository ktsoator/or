package engine

import (
	"fmt"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
)

// toolRuntime owns the session's tool catalog, authorization service, durable
// dispatch boundary, and optional background-task manager. Lifecycle identity
// and transcript storage remain owned by the injected coordinator and journal.
type toolRuntime struct {
	mu sync.RWMutex

	agent       *agent.Agent
	activeTools []tools.Tool
	allTools    []tools.Tool
	toolByName  map[string]tools.Tool
	authorizer  *permission.Service

	taskManager     *tools.TaskManager
	taskUnsubscribe func()
	cwd             string

	journal   *sessionJournal
	lifecycle *lifecycleCoordinator
	recorder  observability.Recorder
	sessionID string

	runPersistenceError    func() error
	recordPersistenceError func(error)
	systemPrompt           func([]tools.Tool) string
	activateSkill          func(string)
	stageTaskStatus        func(string)
	dispatchEvent          func(Event)
	planMode               tools.PlanModeState
}

type toolRuntimeOptions struct {
	cwd                    string
	configuredTools        []tools.Tool
	additionalTools        []tools.Tool
	browser                tools.BrowserController
	asker                  tools.Asker
	skillTool              agent.AgentTool
	skillsAvailable        bool
	permissionMode         permission.Mode
	approver               permission.Approver
	journal                *sessionJournal
	lifecycle              *lifecycleCoordinator
	recorder               observability.Recorder
	sessionID              string
	runPersistenceError    func() error
	recordPersistenceError func(error)
	systemPrompt           func([]tools.Tool) string
	activateSkill          func(string)
	stageTaskStatus        func(string)
	dispatchEvent          func(Event)
	planMode               tools.PlanModeState
}

func newToolRuntime(opts toolRuntimeOptions) (*toolRuntime, error) {
	var catalog []tools.Tool
	var taskManager *tools.TaskManager
	if opts.configuredTools == nil {
		coreTools, coreTasks := tools.CoreTools(opts.cwd)
		taskManager = coreTasks
		catalog = append(coreTools, tools.BrowserTools(opts.cwd, opts.browser)...)
	} else {
		catalog = append([]tools.Tool(nil), opts.configuredTools...)
	}
	catalog = append(catalog, opts.additionalTools...)
	catalog = append(catalog, tools.Tool{
		AgentTool: opts.skillTool,
		AccessFor: tools.InternalAccess,
	})
	if opts.asker != nil {
		catalog = append(catalog, tools.AskUserQuestion(opts.asker))
		catalog = append(catalog, tools.ExitPlanMode(opts.asker, opts.planMode))
	}

	runtime := &toolRuntime{
		activeTools:            toolsWithSkillAvailability(catalog, opts.skillsAvailable),
		allTools:               catalog,
		toolByName:             toolsByName(catalog),
		taskManager:            taskManager,
		cwd:                    opts.cwd,
		journal:                opts.journal,
		lifecycle:              opts.lifecycle,
		recorder:               opts.recorder,
		sessionID:              opts.sessionID,
		runPersistenceError:    opts.runPersistenceError,
		recordPersistenceError: opts.recordPersistenceError,
		systemPrompt:           opts.systemPrompt,
		activateSkill:          opts.activateSkill,
		stageTaskStatus:        opts.stageTaskStatus,
		dispatchEvent:          opts.dispatchEvent,
		planMode:               opts.planMode,
	}
	authorizer, err := permission.NewService(
		opts.cwd,
		opts.permissionMode,
		runtime.observedApprover(opts.approver),
	)
	if err != nil {
		if taskManager != nil {
			taskManager.Shutdown()
		}
		return nil, err
	}
	runtime.authorizer = authorizer
	runtime.subscribeTasks()
	return runtime, nil
}

func (runtime *toolRuntime) bindAgent(bound *agent.Agent) {
	runtime.mu.Lock()
	runtime.agent = bound
	runtime.mu.Unlock()
}

func (runtime *toolRuntime) agentTools() []agent.AgentTool {
	return tools.AgentTools(runtime.activeToolSnapshot())
}

func (runtime *toolRuntime) stableSystemPrompt() string {
	return runtime.systemPrompt(runtime.activeToolSnapshot())
}

func (runtime *toolRuntime) activeToolSnapshot() []tools.Tool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return append([]tools.Tool(nil), runtime.activeTools...)
}

func (runtime *toolRuntime) setSkillAvailable(available bool) {
	runtime.mu.Lock()
	next := toolsWithSkillAvailability(runtime.allTools, available)
	if sameToolNames(runtime.activeTools, next) {
		runtime.mu.Unlock()
		return
	}
	runtime.activeTools = next
	bound := runtime.agent
	active := append([]tools.Tool(nil), next...)
	runtime.mu.Unlock()

	if bound != nil {
		bound.SetTools(tools.AgentTools(active))
		bound.SetSystemPrompt(runtime.systemPrompt(active))
	}
}

func (runtime *toolRuntime) lookup(name string) (tools.Tool, bool) {
	tool, ok := runtime.toolByName[name]
	return tool, ok
}

func (runtime *toolRuntime) toolState(toolCallID string) (toolCorrelationState, bool) {
	return runtime.lifecycle.toolState(toolCallID)
}

func (runtime *toolRuntime) setPermissionMode(mode permission.Mode) {
	runtime.authorizer.SetMode(mode)
}

func (runtime *toolRuntime) beforeToolCall(call agent.BeforeToolCallCtx) (bool, string) {
	runtime.beginObservedTool(call.ToolCall.ID, call.ToolCall.Name)
	if runtime.runPersistenceError() != nil {
		return true, toolCheckpointBlockedMessage
	}
	args, _ := call.Args.(map[string]any)
	var accesses []permission.Access
	if tool, ok := runtime.lookup(call.ToolCall.Name); ok {
		accesses = tool.Accesses(args)
	}
	result, _ := runtime.authorizer.Authorize(call.RunContext, permission.Request{
		ToolCallID: call.ToolCall.ID,
		Tool:       call.ToolCall.Name,
		Args:       args,
		Accesses:   accesses,
	})
	if !result.Allowed {
		return true, result.Reason
	}
	if err := runtime.checkpointToolCall(call); err != nil {
		runtime.recordPersistenceError(err)
		return true, toolCheckpointBlockedMessage
	}
	return false, ""
}

func (runtime *toolRuntime) afterToolCall(call agent.AfterToolCallCtx) *agent.AfterToolCallResult {
	if call.ToolCall.Name == skills.ToolName && !call.Result.Outcome.Failed() {
		if args, ok := call.Args.(map[string]any); ok {
			if name, ok := args["name"].(string); ok {
				runtime.activateSkill(name)
			}
		}
	}
	return nil
}

func (runtime *toolRuntime) subscribeTasks() {
	if runtime.taskManager == nil {
		return
	}
	runtime.taskUnsubscribe = runtime.taskManager.Subscribe(func(state tools.TaskState) {
		if state.Status != tools.TaskRunning {
			runtime.stageTaskStatus(renderTaskStatus(runtime.taskManager.Completed()))
		}
		eventType := TaskCompleted
		if state.Status == tools.TaskRunning {
			eventType = TaskStarted
		}
		runtime.dispatchEvent(Event{
			Type:           eventType,
			BackgroundTask: projectBackgroundTask(state),
		})
	})
}

func (runtime *toolRuntime) close() {
	if runtime.taskUnsubscribe != nil {
		runtime.taskUnsubscribe()
	}
	if runtime.taskManager != nil {
		runtime.taskManager.Shutdown()
	}
}

func (runtime *toolRuntime) backgroundTasks() []BackgroundTask {
	if runtime.taskManager == nil {
		return nil
	}
	states := runtime.taskManager.Snapshot()
	result := make([]BackgroundTask, 0, len(states))
	for _, state := range states {
		result = append(result, projectBackgroundTask(state))
	}
	return result
}

func (runtime *toolRuntime) startTask(command, description string) (tools.TaskInfo, error) {
	if runtime.taskManager == nil {
		return tools.TaskInfo{}, fmt.Errorf("coding: background tasks unavailable")
	}
	return runtime.taskManager.Start(command, description, runtime.cwd)
}

func (runtime *toolRuntime) stopTask(id string) error {
	if runtime.taskManager == nil {
		return fmt.Errorf("%w: %s", tools.ErrTaskNotFound, id)
	}
	return runtime.taskManager.Stop(id)
}

func (runtime *toolRuntime) taskOutput(id string) (TaskOutput, error) {
	if runtime.taskManager == nil {
		return TaskOutput{}, fmt.Errorf("%w: %s", tools.ErrTaskNotFound, id)
	}
	output, err := runtime.taskManager.ReadOutput(id, 0)
	if err != nil {
		return TaskOutput{}, err
	}
	return TaskOutput{Content: output.Content, Truncated: output.Truncated}, nil
}

// toolsByName indexes the complete catalog, including tools currently hidden
// from the model, so authorization and product integrations use one snapshot.
func toolsByName(toolSet []tools.Tool) map[string]tools.Tool {
	result := make(map[string]tools.Tool, len(toolSet))
	for _, tool := range toolSet {
		result[tool.Name()] = tool
	}
	return result
}

func toolsWithSkillAvailability(toolSet []tools.Tool, available bool) []tools.Tool {
	result := make([]tools.Tool, 0, len(toolSet))
	for _, tool := range toolSet {
		if !available && tool.Name() == skills.ToolName {
			continue
		}
		result = append(result, tool)
	}
	return result
}

func sameToolNames(left, right []tools.Tool) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Name() != right[index].Name() {
			return false
		}
	}
	return true
}
