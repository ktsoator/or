package engine

import (
	"context"
	"errors"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/llm"
)

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
	s.context.prepareSkillRefresh()
	s.setSkillToolAvailable(s.context.hasSkills())
	s.context.prepareContextRefresh()
	startedAt := time.Now().UTC()
	started := s.lifecycle.startRun(
		len(s.agent.Snapshot().Messages),
		startedAt,
	)
	runID := started.runID
	turnID := started.turnID
	s.setRunExecutionState(ctx)
	defer func() {
		s.lifecycle.reset()
		s.clearRunExecutionState()
	}()
	s.dispatchEvent(Event{Type: RunStarted, RunID: runID, StartedAt: startedAt})
	s.recorder.Record(observability.Event{
		Name: observability.RunStarted, SessionID: s.sessionID, RunID: runID,
		StartedAt: startedAt, Status: "running",
	})
	s.recordTurnStarted(runID, turnID, startedAt)

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
		s.dropTrailingErrorStep("persistence_failure")
		runErr = checkpointErr
	}
	completedAt := time.Now().UTC()
	terminal := s.lifecycle.finishRun(
		len(s.agent.Snapshot().Messages),
		completedAt,
		runErr,
		checkpointErr,
	)
	persistErr := s.persistRunTerminal(
		ctx,
		terminal,
	)
	// The durable terminal entries above describe execution and are part of the
	// batch that may fail. Diagnostics run after that attempt, so they can expose
	// persistence_failed without claiming that failure was durably committed.
	turnStatus := "completed"
	turnErrorCode := ""
	if finalErr := errors.Join(runErr, persistErr); finalErr != nil {
		turnStatus, turnErrorCode = runFailure(finalErr, checkpointErr, persistErr)
	}
	s.recordTurnTerminal(
		terminal.runID,
		terminal.turnID,
		turnStatus,
		turnErrorCode,
		terminal.turnStartedAt,
		completedAt,
	)
	userMessageIDs, assistantMessageID := s.persistedRunMessageIDs(runID)
	s.dispatchEvent(Event{
		Type:               RunCompleted,
		RunID:              runID,
		Usage:              runUsage,
		StartedAt:          startedAt,
		CompletedAt:        completedAt,
		UserMessageIDs:     userMessageIDs,
		AssistantMessageID: assistantMessageID,
	})
	finalErr := errors.Join(runErr, persistErr)
	s.recordRunTerminal(runID, startedAt, completedAt, runErr, checkpointErr, persistErr)
	return finalErr
}

func (s *Session) persistedRunMessageIDs(runID string) ([]string, string) {
	projection, _, err := s.journal.projectionSnapshot()
	if err != nil {
		return nil, ""
	}

	var userMessageIDs []string
	assistantMessageID := ""
	for _, entry := range projection.Messages {
		if entry.RunID != runID {
			continue
		}
		message, ok := agent.ToLLM(entry.Message)
		if !ok {
			continue
		}
		switch typed := message.(type) {
		case *llm.UserMessage:
			userMessageIDs = append(userMessageIDs, entry.EntryID)
		case *llm.AssistantMessage:
			if typed != nil && (typed.StopReason == llm.StopReasonStop || typed.StopReason == llm.StopReasonLength) {
				assistantMessageID = entry.EntryID
			}
		}
	}
	return userMessageIDs, assistantMessageID
}

func (s *Session) recordRunTerminal(
	runID string,
	startedAt, completedAt time.Time,
	runErr, checkpointErr, persistErr error,
) {
	event := observability.Event{
		Name: observability.RunCompleted, SessionID: s.sessionID, RunID: runID,
		Status: "completed", StartedAt: startedAt, Duration: completedAt.Sub(startedAt),
	}
	if finalErr := errors.Join(runErr, persistErr); finalErr != nil {
		event.Name = observability.RunFailed
		event.Status, event.ErrorCode = runFailure(finalErr, checkpointErr, persistErr)
	}
	s.recorder.Record(event)
}

func runFailure(finalErr, checkpointErr, persistErr error) (status, code string) {
	switch {
	case errors.Is(finalErr, context.Canceled):
		return "cancelled", "context_cancelled"
	case errors.Is(finalErr, context.DeadlineExceeded):
		return "failed", "deadline_exceeded"
	case checkpointErr != nil:
		return "failed", "checkpoint_failed"
	case persistErr != nil:
		return "failed", "persistence_failed"
	default:
		return "failed", "run_failed"
	}
}
