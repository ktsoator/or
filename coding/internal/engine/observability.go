package engine

import (
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/llm"
)

func (s *Session) observeAgentEvent(event agent.AgentEvent) {
	switch event.Type {
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
	message, ok := eventAssistantMessage(event.Message)
	if !ok {
		return "failed", "assistant_message_missing"
	}
	switch message.StopReason {
	case llm.StopReasonAborted:
		return "cancelled", "context_cancelled"
	case llm.StopReasonError:
		if s.runPersistenceError() != nil {
			return "failed", "checkpoint_failed"
		}
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
