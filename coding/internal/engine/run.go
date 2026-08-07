package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/prompttemplate"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/llm"
)

type sessionRunState struct {
	mu sync.RWMutex

	ctx                  context.Context
	startedAt            time.Time
	entryStart           int
	autoCompactAttempted bool
	persistenceErr       error
}

// Prompt starts a run from a text message and optional images, blocking until it
// completes. Newly appended messages are persisted. It returns ErrBusy if a run
// is already in progress.
func (s *Session) Prompt(ctx context.Context, text string, images ...llm.ImageContent) error {
	return s.PromptWithFiles(ctx, text, nil, images...)
}

// PromptWithFiles starts a run with text files that remain product-owned
// context rather than becoming a new LLM SDK content type.
func (s *Session) PromptWithFiles(
	ctx context.Context,
	text string,
	files []AttachedFile,
	images ...llm.ImageContent,
) error {
	return s.run(ctx, func(ctx context.Context) error {
		message, err := s.promptMessage(text, files, images...)
		if err != nil {
			return err
		}
		return s.agent.Prompt(ctx, message)
	})
}

func (s *Session) promptMessage(
	text string,
	files []AttachedFile,
	images ...llm.ImageContent,
) (agent.AgentMessage, error) {
	registry := s.skillRegistry.Snapshot()
	if s.pendingSkills != nil {
		registry = s.pendingSkills
	}
	_, matched, err := registry.ResolveExplicitInvocation(text)
	if err != nil {
		return nil, err
	}
	if matched {
		if activated, ok := registry.ExplicitInvocationSkill(text); ok {
			s.modelContext.StageActivatedSkill(
				activated.Name,
				"",
				skills.FormatActivatedContext(activated),
			)
		}
		return userMessage(
			registry.DisplayExplicitInvocation(text),
			files,
			images,
		), nil
	}
	templates := s.promptTemplates
	if s.promptTemplateLoader != nil {
		templates = prompttemplate.NewRegistry(s.promptTemplateLoader())
		s.promptTemplates = templates
	}
	expanded, matched := templates.ExpandExplicitInvocation(text)
	if !matched {
		return userMessage(text, files, images), nil
	}
	return userMessage(text, files, images, expanded), nil
}

// Continue resumes a run from the current transcript without adding a message.
// It returns ErrBusy if a run is already in progress.
func (s *Session) Continue(ctx context.Context) error {
	return s.run(ctx, s.agent.Continue)
}

// run serializes a single Prompt or Continue invocation. Model-request prefixes
// are checkpointed during the run, and the final assistant plus run metadata
// are flushed when it completes.
func (s *Session) run(ctx context.Context, fn func(context.Context) error) error {
	if !s.runMu.TryLock() {
		return ErrBusy
	}
	defer s.runMu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	// Flush any messages left in memory by an earlier store failure before this
	// run captures its durable starting position. Otherwise their later
	// persistence could be mistaken for messages produced by the new run.
	if err := s.persistNew(ctx); err != nil {
		return err
	}
	s.prepareSkillRefresh()
	s.setSkillToolAvailable(s.currentSkillRegistry().Len() > 0)
	s.prepareContextRefresh()
	startedAt := time.Now().UTC()
	runEntryStart := len(s.snapshotTranscript())
	s.setRunState(ctx, startedAt, runEntryStart)
	defer s.clearRunState()
	s.dispatchEvent(Event{Type: RunStarted, StartedAt: startedAt})

	if s.shouldAutoCompact(s.ContextUsage().UsedTokens) {
		_, _ = s.autoCompact(ctx)
	}

	var runUsage llm.Usage
	unsubscribe := s.agent.Subscribe(func(event agent.AgentEvent) {
		if event.Type != agent.MessageEnd {
			return
		}
		if assistant, ok := eventAssistantMessage(event.Message); ok {
			addUsage(&runUsage, assistant.Usage)
		}
	})
	defer unsubscribe()

	runErr := fn(ctx)
	checkpointErr := s.runPersistenceError()
	if checkpointErr == nil && runErr != nil && !s.trailingContextOverflow() && s.maxRetries > 0 {
		runErr = s.withRetry(ctx, runErr)
		checkpointErr = s.runPersistenceError()
	}
	if checkpointErr == nil && s.trailingContextOverflow() {
		recovered, err := s.recoverContextOverflow(ctx, runErr)
		runErr = err
		checkpointErr = s.runPersistenceError()
		if checkpointErr == nil && recovered && runErr != nil && s.maxRetries > 0 {
			runErr = s.withRetry(ctx, runErr)
			checkpointErr = s.runPersistenceError()
		}
	}
	if checkpointErr != nil {
		// A StreamFn setup failure becomes a synthetic assistant error inside the
		// reusable agent. This error belongs to the persistence layer, not the
		// conversation, so remove it before the final flush and never feed it into
		// model retry or context-overflow recovery.
		s.dropTrailingErrorTurn()
		runErr = checkpointErr
	}
	completedAt := time.Now().UTC()
	persistErr := s.persistNewRun(ctx, runEntryStart, startedAt, completedAt)
	s.dispatchEvent(Event{
		Type:        RunCompleted,
		Usage:       runUsage,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	})
	return errors.Join(runErr, persistErr)
}

func (s *Session) setRunState(ctx context.Context, startedAt time.Time, entryStart int) {
	s.runState.mu.Lock()
	s.runState.ctx = ctx
	s.runState.startedAt = startedAt
	s.runState.entryStart = entryStart
	s.runState.autoCompactAttempted = false
	s.runState.persistenceErr = nil
	s.runState.mu.Unlock()
}

func (s *Session) clearRunState() {
	s.runState.mu.Lock()
	s.runState.ctx = nil
	s.runState.startedAt = time.Time{}
	s.runState.entryStart = 0
	s.runState.autoCompactAttempted = false
	s.runState.persistenceErr = nil
	s.runState.mu.Unlock()
}

func (s *Session) recordRunPersistenceError(err error) {
	if err == nil {
		return
	}
	s.runState.mu.Lock()
	if s.runState.persistenceErr == nil {
		s.runState.persistenceErr = err
	}
	s.runState.mu.Unlock()
}

func (s *Session) runPersistenceError() error {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.persistenceErr
}

func (s *Session) activeRunState() (context.Context, time.Time, int) {
	s.runState.mu.RLock()
	defer s.runState.mu.RUnlock()
	return s.runState.ctx, s.runState.startedAt, s.runState.entryStart
}
