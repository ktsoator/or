package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/capability"
	"github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/prompttemplate"
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
	initialPromptTemplates := opts.PromptTemplates
	if opts.PromptTemplateLoader != nil {
		initialPromptTemplates = opts.PromptTemplateLoader()
	}

	registry := capability.NewRegistry()
	var tasks *tools.TaskManager
	if opts.Tools == nil {
		coreTools, coreTasks := tools.CoreToolsWithTasks(cwd, tools.LocalOps{})
		tasks = coreTasks
		if err := registerTools(registry, "coding.core-tools", coreTools, false); err != nil {
			return nil, err
		}
		if err := registerTools(registry, "coding.browser", tools.BrowserTools(cwd, opts.Browser), false); err != nil {
			return nil, err
		}
	} else if err := registerTools(registry, "coding.configured-tools", opts.Tools, false); err != nil {
		return nil, err
	}
	// Keep one snapshot-aware Skill tool available for request-boundary add/remove
	// changes. It is filtered out of the provider tool set while no Skill exists.
	if err := registerTools(registry, "coding.skills", []tools.Tool{{
		AgentTool: dynamicSkills.Tool(),
		AccessFor: tools.InternalAccess,
	}}, true); err != nil {
		return nil, err
	}
	// The question tool needs a surface that can reach the user, which only the
	// product shell has. A session without one advertises no question tool.
	if opts.Asker != nil {
		if err := registerTools(registry, "coding.ask-user", []tools.Tool{tools.AskUserQuestion(opts.Asker)}, true); err != nil {
			return nil, err
		}
	}
	for _, definition := range opts.Capabilities {
		if err := registry.Register(definition); err != nil {
			return nil, fmt.Errorf("register coding capability %q: %w", definition.Manifest.ID, err)
		}
	}
	toolSet := registry.Tools()
	activeToolSet := toolsWithSkillAvailability(toolSet, initialRegistry.Len() > 0)
	capabilityBeforeToolCall := registry.BeforeToolCall()
	capabilityAfterToolCall := registry.AfterToolCall()

	authorizer, err := permission.NewService(cwd, opts.Policy, opts.Approver)
	if err != nil {
		return nil, err
	}
	journal, seed, entries, err := newSessionJournal(ctx, opts.Store, opts.DetailsStore)
	if err != nil {
		return nil, err
	}

	maxRetries := defaultMaxRetries
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}

	s := &Session{
		journal:              journal,
		tools:                activeToolSet,
		allTools:             toolSet,
		toolByName:           toolsByName(toolSet),
		authorizer:           authorizer,
		tasks:                tasks,
		cwd:                  cwd,
		promptSections:       registry.PromptSections(),
		instructions:         opts.Instructions,
		skillRegistry:        dynamicSkills,
		skillLoader:          opts.SkillLoader,
		skillRevision:        initialRegistry.Revision(),
		promptTemplates:      prompttemplate.NewRegistry(initialPromptTemplates),
		promptTemplateLoader: opts.PromptTemplateLoader,
		maxRetries:           maxRetries,
		contextWindow:        opts.Model.ContextWindow,
		compactor:            opts.Compactor,
	}
	if s.compactor == nil {
		s.compactor = compaction.LLM{
			StreamFn: opts.StreamFn, StreamOptions: opts.StreamOptions,
			GetAPIKey: opts.GetAPIKey,
		}
	}
	baseRendered, baseRevision := s.buildBaseContext()
	s.contextRevision = baseRevision
	s.modelContext = modelcontext.New(
		nextContextEpoch(entries),
		baseRevision,
		baseRendered,
		s.skillRevision,
		s.buildSkillListing(),
	)
	s.modelContext.RestoreActivatedSkills(restoredActivatedSkills(entries))
	if s.tasks != nil {
		s.taskUnsubscribe = s.tasks.Subscribe(func(state tools.TaskState) {
			if state.Status != tools.TaskRunning {
				s.modelContext.StageTaskStatus(renderTaskStatus(s.tasks.Completed()))
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
			if capabilityBeforeToolCall != nil {
				if block, reason := capabilityBeforeToolCall(bc); block {
					return true, reason
				}
			}
			args, _ := bc.Args.(map[string]any)
			var accesses []permission.Access
			if t, ok := s.toolByName[bc.ToolCall.Name]; ok {
				accesses = t.Accesses(args)
			}
			decision, _ := s.authorizer.Authorize(bc.RunContext, permission.Request{
				ToolCallID: bc.ToolCall.ID,
				Tool:       bc.ToolCall.Name,
				Args:       args,
				Accesses:   accesses,
			})
			return decision.Behavior != permission.Allow, decision.Reason
		},
		AfterToolCall: func(ctx agent.AfterToolCallCtx) *agent.AfterToolCallResult {
			override := (*agent.AfterToolCallResult)(nil)
			if capabilityAfterToolCall != nil {
				override = capabilityAfterToolCall(ctx)
			}
			outcome := ctx.Result.Outcome
			if override != nil && override.Outcome != nil {
				outcome = *override.Outcome
			}
			if ctx.ToolCall.Name == skills.ToolName && !outcome.Failed() {
				if args, ok := ctx.Args.(map[string]any); ok {
					if name, ok := args["name"].(string); ok {
						s.activateSkill(name)
					}
				}
			}
			return override
		},
		PrepareNextTurn: s.prepareNextTurn,
	}
	s.agent = agent.New(agentOpts)
	s.journal.captureOutcomes(s.agent)
	s.agent.Subscribe(func(ev agent.AgentEvent) {
		if projected, ok := projectAgentEvent(ev); ok {
			s.dispatchEvent(projected)
		}
	})

	return s, nil
}

func registerTools(registry *capability.Registry, id string, toolSet []tools.Tool, replace bool) error {
	contributions := make([]capability.ToolContribution, len(toolSet))
	for index, tool := range toolSet {
		contributions[index] = capability.ToolContribution{Tool: tool, Replace: replace}
	}
	if err := registry.Register(capability.Definition{
		Manifest: capability.Manifest{ID: id, Version: "1"},
		Tools:    contributions,
	}); err != nil {
		return fmt.Errorf("register coding capability %q: %w", id, err)
	}
	return nil
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
