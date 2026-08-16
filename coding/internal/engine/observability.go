package engine

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

func (s *Session) observeAgentEvent(event agent.AgentEvent) {
	switch event.Type {
	case agent.ToolStart:
		s.beginObservedTool(event.ToolCallID, event.ToolName)
	case agent.ToolEnd:
		s.finishObservedTool(event)
	case agent.TurnEnd:
		completedAt := time.Now().UTC()
		correlation, startedAt := s.finishTurn()
		status, errorCode := s.turnStatus(event)
		s.recorder.Record(observability.Event{
			Name: observability.TurnCompleted, SessionID: s.sessionID,
			RunID: correlation.runID, TurnID: correlation.turnID,
			RequestID: correlation.requestID, Status: status, ErrorCode: errorCode,
			StartedAt: startedAt, Duration: elapsed(startedAt, completedAt),
		})
	}
}

func (s *Session) beginObservedTool(toolCallID, toolName string) {
	startedAt := time.Now().UTC()
	state, started := s.beginTool(toolCallID, toolName, startedAt)
	if !started {
		return
	}
	s.recorder.Record(toolEvent(
		s, state, observability.ToolStarted, "running", "", slog.LevelInfo, startedAt,
	))
}

func (s *Session) finishObservedTool(event agent.AgentEvent) {
	state, found := s.finishTool(event.ToolCallID)
	if !found {
		return
	}
	status := string(event.Result.Outcome.Status)
	if status == "" {
		status = string(agent.ToolOutcomeSuccess)
	}
	name := observability.ToolCompleted
	level := slog.LevelInfo
	errorCode := ""
	if event.Result.Outcome.Failed() {
		name = observability.ToolFailed
		level = slog.LevelError
		errorCode = toolFailureCode(event.Result.Outcome)
	}
	completedAt := time.Now().UTC()
	record := toolEvent(s, state, name, status, errorCode, level, state.startedAt)
	record.Duration = elapsed(state.startedAt, completedAt)
	s.recorder.Record(record)
}

func toolFailureCode(outcome agent.ToolOutcome) string {
	if stableToolErrorCodes[outcome.ErrorCode] {
		return outcome.ErrorCode
	}
	switch outcome.Status {
	case agent.ToolOutcomeCancelled:
		return "tool_cancelled"
	case agent.ToolOutcomeTimeout:
		return "tool_timeout"
	default:
		return "tool_failed"
	}
}

var stableToolErrorCodes = map[string]bool{
	"argument_encoding_failed":          true,
	"background_unavailable":            true,
	"browser_disposition_invalid":       true,
	"browser_inspection_failed":         true,
	"browser_inspection_result_invalid": true,
	"browser_navigation_failed":         true,
	"browser_navigation_timeout":        true,
	"browser_result_invalid":            true,
	"browser_tab_id_invalid":            true,
	"browser_tabs_failed":               true,
	"browser_tabs_result_invalid":       true,
	"browser_unavailable":               true,
	"command_cancelled":                 true,
	"command_execution_failed":          true,
	"command_exit_nonzero":              true,
	"command_start_failed":              true,
	"command_timeout":                   true,
	"invalid_arguments":                 true,
	"invalid_tool_outcome":              true,
	"preview_invalid":                   true,
	"task_not_found":                    true,
	"tool_blocked":                      true,
	"tool_cancelled":                    true,
	"tool_execution_cancelled":          true,
	"tool_execution_failed":             true,
	"tool_execution_timeout":            true,
	"tool_failed":                       true,
	"tool_panicked":                     true,
	"tool_timeout":                      true,
	"tool_unavailable":                  true,
	"unknown_tool":                      true,
}

func toolEvent(
	s *Session,
	state toolCorrelationState,
	name, status, errorCode string,
	level slog.Level,
	startedAt time.Time,
) observability.Event {
	return observability.Event{
		Name: name, Level: level,
		SessionID: s.sessionID, RunID: state.correlation.runID,
		TurnID: state.correlation.turnID, RequestID: state.correlation.requestID,
		ToolCallID: state.toolCallID, ToolName: state.toolName,
		Status: status, ErrorCode: errorCode, StartedAt: startedAt,
	}
}

type observedApprover struct {
	session  *Session
	delegate permission.Approver
}

func (s *Session) observedApprover(delegate permission.Approver) permission.Approver {
	if delegate == nil {
		return nil
	}
	return &observedApprover{session: s, delegate: delegate}
}

func (a *observedApprover) Decide(
	ctx context.Context,
	request permission.ApprovalRequest,
) (permission.ApprovalResponse, error) {
	state, found := a.session.toolState(request.Request.ToolCallID)
	if !found {
		return a.delegate.Decide(ctx, request)
	}
	startedAt := time.Now().UTC()
	a.session.recorder.Record(toolEvent(
		a.session, state, observability.ApprovalStarted,
		"waiting", "", slog.LevelInfo, startedAt,
	))
	response, err := a.delegate.Decide(ctx, request)
	completedAt := time.Now().UTC()
	name := observability.ApprovalCompleted
	status := "allowed"
	errorCode := ""
	level := slog.LevelInfo
	switch {
	case err != nil:
		name = observability.ApprovalFailed
		status = "failed"
		errorCode = "approval_failed"
		level = slog.LevelError
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
			errorCode = "context_cancelled"
		} else if errors.Is(err, context.DeadlineExceeded) {
			errorCode = "deadline_exceeded"
		}
	case response.Choice == permission.Reject:
		status = "denied"
	case response.Choice != permission.AllowOnce:
		name = observability.ApprovalFailed
		status = "failed"
		errorCode = "invalid_approval_choice"
		level = slog.LevelError
	}
	record := toolEvent(a.session, state, name, status, errorCode, level, startedAt)
	record.Duration = elapsed(startedAt, completedAt)
	a.session.recorder.Record(record)
	return response, err
}

func (s *Session) beginObservedTurn() {
	startedAt := time.Now().UTC()
	turnID := observability.NewID("turn")
	runID := s.beginTurn(turnID, startedAt)
	s.recorder.Record(observability.Event{
		Name: observability.TurnStarted, SessionID: s.sessionID,
		RunID: runID, TurnID: turnID, Status: "running", StartedAt: startedAt,
	})
}

func (s *Session) turnStatus(event agent.AgentEvent) (status, errorCode string) {
	if s.runPersistenceError() != nil {
		return "failed", "checkpoint_failed"
	}
	message, ok := eventAssistantMessage(event.Message)
	if !ok {
		return "failed", "assistant_message_missing"
	}
	switch message.StopReason {
	case llm.StopReasonAborted:
		return "cancelled", "context_cancelled"
	case llm.StopReasonError:
		return "failed", "provider_request_failed"
	default:
		return "completed", ""
	}
}

func (s *Session) recordTurnDiscarded(reason string) {
	correlation := s.lastTurnCorrelation()
	if correlation.turnID == "" {
		return
	}
	s.recorder.Record(observability.Event{
		Name: observability.TurnDiscarded, SessionID: s.sessionID,
		RunID: correlation.runID, TurnID: correlation.turnID,
		RequestID: correlation.requestID,
		Status:    "discarded", Reason: reason,
	})
}

func usageEventFields(event *observability.Event, usage llm.Usage) {
	event.InputTokens = usage.Input
	event.InputUnknown = usage.InputUnknown
	event.OutputTokens = usage.Output
	event.CacheReadTokens = usage.CacheRead
	event.CacheWriteTokens = usage.CacheWrite
	event.TotalTokens = usage.TotalTokens
	if event.TotalTokens == 0 {
		event.TotalTokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	}
	event.CostInput = usage.Cost.Input
	event.CostOutput = usage.Cost.Output
	event.CostCacheRead = usage.Cost.CacheRead
	event.CostCacheWrite = usage.Cost.CacheWrite
	event.CostTotal = usage.Cost.Total
}
