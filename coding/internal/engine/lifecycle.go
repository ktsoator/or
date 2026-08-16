package engine

import (
	"context"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/transcript"
)

func (s *Session) queueRunLifecycleStart(runID, turnID string, startedAt time.Time) {
	messageIndex := len(s.agent.Snapshot().Messages)
	runStart := transcript.NewRunStart(runID)
	turnStart := transcript.NewTurnStart(runID, turnID)
	runStart.Timestamp = startedAt.UTC()
	turnStart.Timestamp = startedAt.UTC()
	s.queueLifecycle(
		positionedJournalEntry{
			messageIndex: messageIndex,
			entry:        runStart,
		},
		positionedJournalEntry{
			messageIndex: messageIndex,
			entry:        turnStart,
		},
	)
}

func (s *Session) queueStepEnd(
	state stepCorrelationState,
	status transcript.LifecycleStatus,
	reason string,
) {
	if !state.lifecycleDurable {
		return
	}
	messageIndex := len(s.agent.Snapshot().Messages)
	s.queueLifecycle(positionedJournalEntry{
		messageIndex: messageIndex,
		entry: transcript.NewStepEnd(
			state.runID,
			state.lifecycleTurnID,
			state.stepID,
			status,
			reason,
		),
	})
}

func (s *Session) queueFollowUpTurn() {
	messageIndex := len(s.agent.Snapshot().Messages)
	nextTurnID := observability.NewID("turn")
	startedAt := time.Now().UTC()
	runID, previousTurnID, previousStartedAt := s.transitionLifecycleTurn(nextTurnID, startedAt)
	if runID == "" || previousTurnID == "" {
		return
	}
	s.recordTurnTerminal(
		runID, previousTurnID, "completed", "", previousStartedAt, startedAt,
	)
	s.recordTurnStarted(runID, nextTurnID, startedAt)
	turnEnd := transcript.NewTurnEnd(
		runID,
		previousTurnID,
		transcript.LifecycleCompleted,
		"",
	)
	turnStart := transcript.NewTurnStart(runID, nextTurnID)
	turnEnd.Timestamp = startedAt
	turnStart.Timestamp = startedAt
	s.queueLifecycle(
		positionedJournalEntry{
			messageIndex: messageIndex,
			entry:        turnEnd,
		},
		positionedJournalEntry{
			messageIndex: messageIndex,
			entry:        turnStart,
		},
	)
}

func (s *Session) closeOpenSteps(status transcript.LifecycleStatus, reason string) {
	for _, step := range s.finishOpenSteps() {
		s.queueStepEnd(step, status, reason)
	}
}

func positionedLifecycle(
	messageIndex int,
	entries ...transcript.Entry,
) []positionedJournalEntry {
	result := make([]positionedJournalEntry, len(entries))
	for index, entry := range entries {
		result[index] = positionedJournalEntry{messageIndex: messageIndex, entry: entry}
	}
	return result
}

func lifecycleTerminal(
	status, reason string,
) (transcript.LifecycleStatus, string) {
	switch status {
	case "completed":
		return transcript.LifecycleCompleted, ""
	case "cancelled":
		return transcript.LifecycleCancelled, reason
	default:
		return transcript.LifecycleFailed, reason
	}
}

func (s *Session) persistPendingLifecycle(ctx context.Context) error {
	pending := s.pendingLifecycle()
	if len(pending) == 0 {
		return s.journal.persistMessages(
			ctx,
			s.agent.Snapshot().Messages,
			nil,
			nil,
		)
	}
	err := s.journal.persistMessages(
		ctx,
		s.agent.Snapshot().Messages,
		nil,
		pending,
	)
	if err == nil {
		s.clearPendingLifecycle(len(pending))
	}
	return err
}
