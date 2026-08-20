package engine

import (
	"context"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type agentLifecycleDecision struct {
	followUp     lifecycleTurnTransition
	stepComplete lifecycleStepCompleted
}

// coordinateAgentLifecycle translates reusable-agent facts into lifecycle
// transitions. Observability consumes the returned decision but never drives it.
func (s *Session) coordinateAgentLifecycle(event agent.AgentEvent) agentLifecycleDecision {
	now := time.Now().UTC()
	messageIndex := len(s.agent.Snapshot().Messages)
	switch event.Type {
	case agent.FollowUpStart:
		return agentLifecycleDecision{
			followUp: s.lifecycle.claimFollowUp(messageIndex, now),
		}
	case agent.TurnEnd:
		status, errorCode := s.stepStatus(event)
		return agentLifecycleDecision{
			stepComplete: s.lifecycle.completeStep(
				messageIndex,
				now,
				status,
				errorCode,
			),
		}
	default:
		return agentLifecycleDecision{}
	}
}

func (s *Session) stepStatus(event agent.AgentEvent) (status, errorCode string) {
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

func (s *Session) persistPendingLifecycle(ctx context.Context) error {
	pending := s.lifecycle.pendingEntries()
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
		s.lifecycle.commitPending(len(pending))
	}
	return err
}

func (s *Session) correlateVisibleEvent(event *Event) {
	if event == nil || event.ProviderRequestID != "" {
		return
	}
	switch event.Type {
	case TextDelta, ThinkingDelta,
		ToolInputStarted, ToolInputDelta, ToolInputCompleted,
		ToolStarted, ToolFinished, MessageCompleted:
	default:
		return
	}
	if event.ToolCallID != "" {
		if tool, ok := s.toolRuntime.toolState(event.ToolCallID); ok {
			event.ProviderRequestID = tool.correlation.requestID
			return
		}
	}
	event.ProviderRequestID = s.lifecycle.activeRequest().requestID
}
