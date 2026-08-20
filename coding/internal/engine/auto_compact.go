package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const autoCompactDivisor int64 = 5

// shouldAutoCompact reserves one fifth of the model window for the next turn.
// A zero usage or window means the provider has not supplied enough information
// to make a safe proactive decision; overflow recovery still remains available.
func (s *Session) shouldAutoCompact(usedTokens int64) bool {
	contextWindow := s.agent.Snapshot().Model.ContextWindow
	if usedTokens <= 0 || contextWindow <= 0 {
		return false
	}
	threshold := contextWindow - contextWindow/autoCompactDivisor
	return usedTokens >= threshold
}

// prepareNextTurn runs after a tool turn or before queued work continues. It
// installs the compacted projection into both the long-lived Agent and the
// current loop, so a long single run can reclaim context without restarting.
func (s *Session) prepareNextTurn(turn agent.TurnCtx) *agent.TurnUpdate {
	if s.execution.persistenceError() != nil {
		return nil
	}
	if len(turn.ToolResults) == 0 && !s.agent.HasQueuedMessages() {
		return nil
	}
	if !s.shouldAutoCompact(usageTokens(turn.Message.Usage)) {
		return nil
	}
	ctx := s.execution.runContext()
	if ctx == nil {
		return nil
	}
	compacted, err := s.autoCompact(ctx)
	if err != nil || !compacted {
		return nil
	}

	next := turn.Context
	next.Messages = s.agent.Snapshot().Messages
	return &agent.TurnUpdate{Context: &next}
}

// autoCompact performs at most one real compaction attempt per run. A history
// that is still too short does not consume the attempt, because later tool
// output may make overflow recovery possible.
func (s *Session) autoCompact(ctx context.Context) (bool, error) {
	if !s.execution.claimAutoCompaction() {
		return false, nil
	}
	return s.autoCompactClaimed(ctx)
}

func (s *Session) autoCompactClaimed(ctx context.Context) (bool, error) {
	_, err := s.compactLocked(ctx, "", true)
	if IsNothingToCompact(err) {
		s.execution.releaseAutoCompactionClaim()
	}
	return err == nil, err
}

func (s *Session) recoverContextOverflow(ctx context.Context, original error) (bool, error) {
	overflowErr := original
	if overflowErr == nil {
		overflowErr = errors.New("coding: model context overflow")
	}
	if !s.execution.claimAutoCompaction() {
		return false, overflowErr
	}
	contextWindow := s.agent.Snapshot().Model.ContextWindow

	s.dropTrailingAssistantStep(
		"context_overflow",
		func(message *llm.AssistantMessage) bool {
			return llm.IsContextOverflow(*message, contextWindow)
		},
	)
	compacted, err := s.autoCompactClaimed(ctx)
	if err != nil {
		return true, errors.Join(overflowErr, fmt.Errorf("automatic context compaction: %w", err))
	}
	if !compacted {
		return true, overflowErr
	}
	return true, s.agent.Continue(ctx)
}

func (s *Session) trailingContextOverflow() bool {
	state := s.agent.Snapshot()
	if len(state.Messages) == 0 {
		return false
	}
	assistant := asAssistant(state.Messages[len(state.Messages)-1])
	return assistant != nil && llm.IsContextOverflow(
		*assistant,
		state.Model.ContextWindow,
	)
}
