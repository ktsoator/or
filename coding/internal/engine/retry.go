package engine

import (
	"context"
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// Retry defaults. The provider SDK already retries request-level transient
// failures (HTTP 429/5xx, connection errors); this app-level retry is a thin
// safety net above it, catching stream-level failures and errors that survive
// the SDK's own retries.
const (
	defaultMaxRetries = 2
	retryBaseDelay    = 750 * time.Millisecond
	retryMaxDelay     = 8 * time.Second
)

// transientSignals are substrings that mark a failed turn's error as worth
// retrying. They cover rate limiting, upstream 5xx, and dropped connections —
// failures that usually clear on another attempt. Context overflow and user
// aborts are deliberately excluded: overflow needs compaction, and an abort is
// intentional.
var transientSignals = []string{
	"429", "too many requests", "rate limit",
	"500", "502", "503", "504",
	"overloaded", "temporarily", "unavailable", "try again",
	"timeout", "timed out", "deadline",
	"connection", "reset", "eof", "broken pipe",
}

// withRetry starts another Step in the active Turn while the preceding Step's
// error looks transient. It drops the failed assistant message so model context
// ends on a user or tool-result message, waits a growing backoff, then resumes
// with Continue. It gives up after maxRetries attempts or on the first
// non-transient error.
func (s *Session) withRetry(ctx context.Context, err error) error {
	for attempt := 1; err != nil && attempt <= s.maxRetries; attempt++ {
		last := lastAssistant(s.agent.Snapshot().Messages)
		if last == nil || !s.isRetryable(*last) {
			break
		}
		s.dropTrailingErrorStep("retry")
		if !sleepCtx(ctx, backoff(attempt)) {
			break // cancelled during backoff
		}
		err = s.agent.Continue(ctx)
	}
	return err
}

// isRetryable reports whether a failed assistant Step should be retried.
func (s *Session) isRetryable(msg llm.AssistantMessage) bool {
	if msg.StopReason != llm.StopReasonError {
		return false // a clean stop or a user abort is not retryable
	}
	if llm.IsContextOverflow(msg, s.agent.Snapshot().Model.ContextWindow) {
		return false // needs compaction, not a retry
	}
	text := strings.ToLower(msg.ErrorMessage)
	for _, signal := range transientSignals {
		if strings.Contains(text, signal) {
			return true
		}
	}
	return false
}

// dropTrailingErrorStep removes a trailing failed assistant message so Continue
// can resume from the preceding user or tool-result message.
func (s *Session) dropTrailingErrorStep(reason string) {
	s.dropTrailingAssistantStep(reason, func(message *llm.AssistantMessage) bool {
		return message.StopReason == llm.StopReasonError
	})
}

// dropTrailingAssistantStep applies one coordinator discard decision before
// removing a synthetic or recoverable assistant failure from model context.
func (s *Session) dropTrailingAssistantStep(
	reason string,
	shouldDrop func(*llm.AssistantMessage) bool,
) {
	msgs := s.agent.Snapshot().Messages
	n := len(msgs)
	if n == 0 {
		return
	}
	if assistant := asAssistant(msgs[n-1]); assistant != nil && shouldDrop(assistant) {
		discarded := s.lifecycle.discardLastStep(n-1, reason)
		s.recordStepDiscarded(discarded)
		s.dispatchEvent(Event{Type: TurnDiscarded})
		s.agent.SetMessages(msgs[:n-1])
	}
}

// backoff returns the delay before the given retry attempt, doubling from the
// base delay and capped at retryMaxDelay.
func backoff(attempt int) time.Duration {
	d := retryBaseDelay << (attempt - 1)
	if d > retryMaxDelay {
		return retryMaxDelay
	}
	return d
}

// sleepCtx waits for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// lastAssistant returns the most recent assistant message in the transcript, or
// nil if there is none.
func lastAssistant(messages []agent.AgentMessage) *llm.AssistantMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if a := asAssistant(messages[i]); a != nil {
			return a
		}
	}
	return nil
}

// asAssistant unwraps an AgentMessage into an llm assistant message, or nil.
func asAssistant(m agent.AgentMessage) *llm.AssistantMessage {
	llmMsg, ok := agent.ToLLM(m)
	if !ok {
		return nil
	}
	a, _ := llmMsg.(*llm.AssistantMessage)
	return a
}
