package engine

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/observability"
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

	journal, seed, entries, err := newSessionJournal(ctx, opts.Store)
	if err != nil {
		return nil, err
	}

	maxRetries := defaultMaxRetries
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}
	contextState := newContextManager(
		cwd,
		opts.Instructions,
		dynamicSkills,
		opts.SkillLoader,
		entries,
	)

	s := &Session{
		journal:    journal,
		sessionID:  opts.SessionID,
		recorder:   observability.OrDiscard(opts.Recorder),
		context:    contextState,
		maxRetries: maxRetries,
		compactor:  opts.Compactor,
	}
	toolState, err := newToolRuntime(toolRuntimeOptions{
		cwd:                    cwd,
		model:                  opts.Model,
		configuredTools:        opts.Tools,
		additionalTools:        opts.AdditionalTools,
		browser:                opts.Browser,
		asker:                  opts.Asker,
		skillTool:              dynamicSkills.Tool(),
		skillsAvailable:        initialRegistry.Len() > 0,
		permissionMode:         opts.PermissionMode,
		approver:               opts.Approver,
		journal:                journal,
		lifecycle:              &s.lifecycle,
		recorder:               s.recorder,
		sessionID:              s.sessionID,
		runPersistenceError:    s.execution.persistenceError,
		recordPersistenceError: s.execution.recordPersistenceError,
		systemPrompt: func(toolSet []tools.Tool) string {
			return contextState.systemPrompt(toolSet, s.PlanModeActive())
		},
		activateSkill:   contextState.activateSkill,
		stageTaskStatus: contextState.stageTaskStatus,
		dispatchEvent:   s.dispatchEvent,
		planMode:        s,
	})
	if err != nil {
		return nil, err
	}
	s.toolRuntime = toolState
	if s.compactor == nil {
		s.compactor = compaction.LLM{
			StreamFn: opts.StreamFn, StreamOptions: opts.StreamOptions,
			GetAPIKey: opts.GetAPIKey,
		}
	}
	agentOpts := agent.Options{
		SystemPrompt:   s.toolRuntime.stableSystemPrompt(),
		Model:          opts.Model,
		ThinkingLevel:  opts.ThinkingLevel,
		Tools:          s.toolRuntime.agentTools(),
		Messages:       seed,
		StreamOptions:  opts.StreamOptions,
		StreamFn:       s.modelStreamFn(opts.StreamFn),
		GetAPIKey:      opts.GetAPIKey,
		BeforeToolCall: s.toolRuntime.beforeToolCall,
		AfterToolCall:  s.toolRuntime.afterToolCall,
		ShouldStopAfterStep: func(agent.StepCtx) bool {
			return s.execution.persistenceError() != nil
		},
		PrepareNextStep: s.prepareNextStep,
	}
	s.agent = agent.New(agentOpts)
	s.toolRuntime.bindAgent(s.agent)
	s.journal.captureOutcomes(s.agent)
	s.agent.Subscribe(func(ev agent.AgentEvent) {
		projected, visible := projectAgentEvent(ev)
		if visible {
			// Correlate before observeAgentEvent removes terminal tool state.
			s.correlateVisibleEvent(&projected)
		}
		lifecycleDecision := s.coordinateAgentLifecycle(ev)
		s.observeAgentEvent(ev, lifecycleDecision)
		if turnStarted, ok := turnStartedEvent(lifecycleDecision); ok {
			s.dispatchEvent(turnStarted)
		}
		if visible {
			if projected.Type == MessageCompleted {
				projected.ContextUsage = s.ContextUsage()
			}
			s.dispatchEvent(projected)
		}
	})

	return s, nil
}
