package engine

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/contextprojection"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/requestsnapshot"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
)

// New builds a Session. When a Store is configured, its transcript is loaded and
// used to seed the agent, so the session resumes where it left off.
func New(ctx context.Context, opts Options) (*Session, error) {
	cwd := opts.Cwd
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd = wd
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	initialSkills := opts.Skills
	if opts.SkillLoader != nil {
		initialSkills = opts.SkillLoader()
	}
	initialRegistry := skills.NewRegistry(initialSkills)
	dynamicSkills := skills.NewDynamicRegistry(initialRegistry)
	var toolSet []tools.Tool
	var tasks *tools.TaskManager
	if opts.Tools == nil {
		coreTools, coreTasks := tools.CoreTools(cwd)
		tasks = coreTasks
		toolSet = append(coreTools, tools.BrowserTools(cwd, opts.Browser)...)
	} else {
		toolSet = append([]tools.Tool(nil), opts.Tools...)
	}
	toolSet = append(toolSet, opts.AdditionalTools...)
	// Keep one snapshot-aware Skill tool available for request-boundary add/remove
	// changes. It is filtered out of the provider tool set while no Skill exists.
	toolSet = append(toolSet, tools.Tool{
		AgentTool: dynamicSkills.Tool(),
		AccessFor: tools.InternalAccess,
	})
	// The question tool needs a surface that can reach the user, which only the
	// product shell has. A session without one advertises no question tool.
	if opts.Asker != nil {
		toolSet = append(toolSet, tools.AskUserQuestion(opts.Asker))
	}
	activeToolSet := toolsWithSkillAvailability(toolSet, initialRegistry.Len() > 0)

	journal, seed, entries, err := newSessionJournal(ctx, opts.Store)
	if err != nil {
		return nil, err
	}

	maxRetries := defaultMaxRetries
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}

	s := &Session{
		journal:          journal,
		sessionID:        opts.SessionID,
		recorder:         observability.OrDiscard(opts.Recorder),
		requestSnapshots: requestsnapshot.OrDiscard(opts.RequestSnapshots),
		tools:            activeToolSet,
		allTools:         toolSet,
		toolByName:       toolsByName(toolSet),
		tasks:            tasks,
		cwd:              cwd,
		instructions:     opts.Instructions,
		skillRegistry:    dynamicSkills,
		skillLoader:      opts.SkillLoader,
		skillRevision:    initialRegistry.Revision(),
		maxRetries:       maxRetries,
		contextWindow:    opts.Model.ContextWindow,
		compactor:        opts.Compactor,
	}
	authorizer, err := permission.NewService(
		cwd,
		opts.PermissionMode,
		s.observedApprover(opts.Approver),
	)
	if err != nil {
		return nil, err
	}
	s.authorizer = authorizer
	if s.compactor == nil {
		s.compactor = compaction.LLM{
			StreamFn: opts.StreamFn, StreamOptions: opts.StreamOptions,
			GetAPIKey: opts.GetAPIKey,
		}
	}
	baseRendered, baseRevision := s.buildBaseContext()
	s.contextRevision = baseRevision
	s.contextProjection = contextprojection.New(
		nextContextEpoch(entries),
		baseRevision,
		baseRendered,
		s.skillRevision,
		s.buildSkillListing(),
	)
	s.contextProjection.RestoreActivatedSkills(restoredActivatedSkills(entries))
	if s.tasks != nil {
		s.taskUnsubscribe = s.tasks.Subscribe(func(state tools.TaskState) {
			if state.Status != tools.TaskRunning {
				s.contextProjection.StageTaskStatus(renderTaskStatus(s.tasks.Completed()))
			}
			eventType := TaskCompleted
			if state.Status == tools.TaskRunning {
				eventType = TaskStarted
			}
			s.dispatchEvent(Event{
				Type:           eventType,
				BackgroundTask: projectBackgroundTask(state),
			})
		})
	}

	agentOpts := agent.Options{
		SystemPrompt:  s.buildSystemPrompt(opts.Instructions),
		Model:         opts.Model,
		ThinkingLevel: opts.ThinkingLevel,
		Tools:         tools.AgentTools(activeToolSet),
		Messages:      seed,
		StreamOptions: opts.StreamOptions,
		StreamFn:      s.modelStreamFn(opts.StreamFn),
		GetAPIKey:     opts.GetAPIKey,
		BeforeToolCall: func(bc agent.BeforeToolCallCtx) (bool, string) {
			s.beginObservedTool(bc.ToolCall.ID, bc.ToolCall.Name)
			if s.runPersistenceError() != nil {
				return true, toolCheckpointBlockedMessage
			}
			args, _ := bc.Args.(map[string]any)
			var accesses []permission.Access
			if t, ok := s.toolByName[bc.ToolCall.Name]; ok {
				accesses = t.Accesses(args)
			}
			result, _ := s.authorizer.Authorize(bc.RunContext, permission.Request{
				ToolCallID: bc.ToolCall.ID,
				Tool:       bc.ToolCall.Name,
				Args:       args,
				Accesses:   accesses,
			})
			if !result.Allowed {
				return true, result.Reason
			}
			if err := s.checkpointToolCall(bc); err != nil {
				s.recordRunPersistenceError(err)
				return true, toolCheckpointBlockedMessage
			}
			return false, ""
		},
		AfterToolCall: func(ctx agent.AfterToolCallCtx) *agent.AfterToolCallResult {
			if ctx.ToolCall.Name == skills.ToolName && !ctx.Result.Outcome.Failed() {
				if args, ok := ctx.Args.(map[string]any); ok {
					if name, ok := args["name"].(string); ok {
						s.activateSkill(name)
					}
				}
			}
			return nil
		},
		ShouldStopAfterTurn: func(agent.TurnCtx) bool {
			return s.runPersistenceError() != nil
		},
		PrepareNextTurn: s.prepareNextTurn,
	}
	s.agent = agent.New(agentOpts)
	s.journal.captureOutcomes(s.agent)
	s.agent.Subscribe(func(ev agent.AgentEvent) {
		projected, visible := projectAgentEvent(ev)
		if visible {
			// Correlate before observeAgentEvent removes terminal tool state.
			s.correlateVisibleEvent(&projected)
		}
		s.observeAgentEvent(ev)
		if visible {
			if projected.Type == MessageCompleted {
				projected.ContextUsage = s.ContextUsage()
			}
			s.dispatchEvent(projected)
		}
	})

	return s, nil
}

// toolsByName indexes the tool set by advertised name for access description.
func toolsByName(toolSet []tools.Tool) map[string]tools.Tool {
	m := make(map[string]tools.Tool, len(toolSet))
	for _, t := range toolSet {
		m[t.Name()] = t
	}
	return m
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
