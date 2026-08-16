package engine

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/transcript"
)

const toolCheckpointBlockedMessage = "tool execution blocked because its dispatch checkpoint could not be persisted"

// checkpointToolCall persists the complete message prefix and one authorized
// dispatch intent before the tool body can run.
func (s *Session) checkpointToolCall(call agent.BeforeToolCallCtx) error {
	if err := s.runPersistenceError(); err != nil {
		return err
	}
	arguments, err := json.Marshal(call.Args)
	if err != nil {
		checkpointErr := fmt.Errorf("coding: encode tool call checkpoint: %w", err)
		s.recordToolCheckpoint(call, time.Now().UTC(), checkpointErr)
		return checkpointErr
	}

	startedAt := time.Now().UTC()
	entry := transcript.NewToolCall(transcript.ToolCall{
		ToolCallID: call.ToolCall.ID,
		ToolName:   call.ToolCall.Name,
		Arguments:  arguments,
	})
	err = s.journal.persistMessages(
		call.RunContext,
		s.agent.Snapshot().Messages,
		nil,
		nil,
		"",
		0,
		time.Time{},
		time.Time{},
		entry,
	)
	if err != nil {
		err = fmt.Errorf("coding: persist tool call checkpoint: %w", err)
	}
	s.recordToolCheckpoint(call, startedAt, err)
	return err
}

func (s *Session) recordToolCheckpoint(
	call agent.BeforeToolCallCtx,
	startedAt time.Time,
	err error,
) {
	correlation := s.activeRequestCorrelation()
	event := observability.Event{
		Name:      observability.CheckpointCompleted,
		SessionID: s.sessionID, RunID: correlation.runID,
		TurnID: correlation.turnID, RequestID: correlation.requestID,
		StepID:     correlation.stepID,
		ToolCallID: call.ToolCall.ID, ToolName: call.ToolCall.Name,
		Status: "completed", Reason: "tool_dispatch",
		StartedAt: startedAt, Duration: time.Since(startedAt),
	}
	if err != nil {
		event.Name = observability.CheckpointFailed
		event.Level = slog.LevelError
		event.Status = "failed"
		event.ErrorCode = "checkpoint_persist_failed"
	}
	s.recorder.Record(event)
}
