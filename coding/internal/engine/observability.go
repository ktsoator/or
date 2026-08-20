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

func (s *Session) observeAgentEvent(
	event agent.AgentEvent,
	decision agentLifecycleDecision,
) {
	switch event.Type {
	case agent.FollowUpStart:
		transition := decision.followUp
		if transition.runID == "" {
			return
		}
		s.recordTurnTerminal(
			transition.runID,
			transition.previousTurnID,
			"completed",
			"",
			transition.previousStartedAt,
			transition.nextStartedAt,
		)
		s.recordTurnStarted(
			transition.runID,
			transition.nextTurnID,
			transition.nextStartedAt,
		)
	case agent.ToolStart:
		s.toolRuntime.beginObservedTool(event.ToolCallID, event.ToolName)
	case agent.ToolEnd:
		s.toolRuntime.finishObservedTool(event)
	case agent.TurnEnd:
		completed := decision.stepComplete
		step := completed.step
		if step.stepID == "" {
			return
		}
		correlation := requestCorrelation{
			runID: step.runID, turnID: step.lifecycleTurnID,
			stepID: step.stepID, requestID: step.requestID,
		}
		s.recorder.Record(observability.Event{
			Name: observability.StepCompleted, SessionID: s.sessionID,
			RunID: correlation.runID, TurnID: correlation.turnID,
			StepID:    correlation.stepID,
			RequestID: correlation.requestID,
			Status:    completed.status, ErrorCode: completed.errorCode,
			StartedAt: step.startedAt,
			Duration:  elapsed(step.startedAt, completed.completedAt),
		})
	}
}

func (runtime *toolRuntime) beginObservedTool(toolCallID, toolName string) {
	startedAt := time.Now().UTC()
	state, started := runtime.lifecycle.beginTool(toolCallID, toolName, startedAt)
	if !started {
		return
	}
	runtime.recorder.Record(toolEvent(
		runtime.sessionID, state,
		observability.ToolStarted, "running", "", slog.LevelInfo, startedAt,
	))
}

func (runtime *toolRuntime) finishObservedTool(event agent.AgentEvent) {
	state, found := runtime.lifecycle.finishTool(event.ToolCallID)
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
	record := toolEvent(
		runtime.sessionID,
		state,
		name,
		status,
		errorCode,
		level,
		state.startedAt,
	)
	record.Duration = elapsed(state.startedAt, completedAt)
	runtime.recorder.Record(record)
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
	sessionID string,
	state toolCorrelationState,
	name, status, errorCode string,
	level slog.Level,
	startedAt time.Time,
) observability.Event {
	return observability.Event{
		Name: name, Level: level,
		SessionID: sessionID, RunID: state.correlation.runID,
		TurnID: state.correlation.turnID, RequestID: state.correlation.requestID,
		StepID:     state.correlation.stepID,
		ToolCallID: state.toolCallID, ToolName: state.toolName,
		Status: status, ErrorCode: errorCode, StartedAt: startedAt,
	}
}

type observedApprover struct {
	runtime  *toolRuntime
	delegate permission.Approver
}

func (runtime *toolRuntime) observedApprover(delegate permission.Approver) permission.Approver {
	if delegate == nil {
		return nil
	}
	return &observedApprover{runtime: runtime, delegate: delegate}
}

func (a *observedApprover) Decide(
	ctx context.Context,
	request permission.ApprovalRequest,
) (permission.ApprovalResponse, error) {
	state, found := a.runtime.toolState(request.Request.ToolCallID)
	if !found {
		return a.delegate.Decide(ctx, request)
	}
	startedAt := time.Now().UTC()
	a.runtime.recorder.Record(toolEvent(
		a.runtime.sessionID, state, observability.ApprovalStarted,
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
	record := toolEvent(
		a.runtime.sessionID,
		state,
		name,
		status,
		errorCode,
		level,
		startedAt,
	)
	record.Duration = elapsed(startedAt, completedAt)
	a.runtime.recorder.Record(record)
	return response, err
}

func (s *Session) recordTurnStarted(runID, turnID string, startedAt time.Time) {
	s.recorder.Record(observability.Event{
		Name: observability.TurnStarted, SessionID: s.sessionID,
		RunID: runID, TurnID: turnID, Status: "running", StartedAt: startedAt,
	})
}

func (s *Session) recordTurnTerminal(
	runID, turnID, status, errorCode string,
	startedAt, completedAt time.Time,
) {
	if runID == "" || turnID == "" {
		return
	}
	s.recorder.Record(observability.Event{
		Name: observability.TurnCompleted, SessionID: s.sessionID,
		RunID: runID, TurnID: turnID, Status: status, ErrorCode: errorCode,
		StartedAt: startedAt, Duration: elapsed(startedAt, completedAt),
	})
}

func (s *Session) recordStepDiscarded(discarded lifecycleStepDiscarded) {
	correlation := discarded.correlation
	if correlation.stepID == "" {
		return
	}
	s.recorder.Record(observability.Event{
		Name: observability.StepDiscarded, SessionID: s.sessionID,
		RunID: correlation.runID, TurnID: correlation.turnID,
		StepID:    correlation.stepID,
		RequestID: correlation.requestID,
		Status:    "discarded", Reason: discarded.reason,
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
