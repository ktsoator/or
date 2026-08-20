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
func (runtime *toolRuntime) checkpointToolCall(call agent.BeforeToolCallCtx) error {
	if err := runtime.runPersistenceError(); err != nil {
		return err
	}
	arguments, err := json.Marshal(call.Args)
	if err != nil {
		checkpointErr := fmt.Errorf("coding: encode tool call checkpoint: %w", err)
		runtime.recordToolCheckpoint(call, time.Now().UTC(), checkpointErr)
		return checkpointErr
	}

	startedAt := time.Now().UTC()
	entry := transcript.NewToolCall(transcript.ToolCall{
		ToolCallID: call.ToolCall.ID,
		ToolName:   call.ToolCall.Name,
		Arguments:  arguments,
	})
	err = runtime.journal.persistMessages(
		call.RunContext,
		runtime.agent.Snapshot().Messages,
		nil,
		nil,
		entry,
	)
	if err != nil {
		err = fmt.Errorf("coding: persist tool call checkpoint: %w", err)
	}
	runtime.recordToolCheckpoint(call, startedAt, err)
	return err
}

func (runtime *toolRuntime) recordToolCheckpoint(
	call agent.BeforeToolCallCtx,
	startedAt time.Time,
	err error,
) {
	correlation := runtime.lifecycle.activeRequest()
	event := observability.Event{
		Name:      observability.CheckpointCompleted,
		SessionID: runtime.sessionID, RunID: correlation.runID,
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
	runtime.recorder.Record(event)
}
