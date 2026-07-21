package coding

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/compaction"
	"github.com/ktsoator/or/coding/transcript"
	"github.com/ktsoator/or/llm"
)

const defaultKeepRecentTokens int64 = 20_000

type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int64
	TokensAfter      int64
	Usage            llm.Usage
	Provider         string
	Model            string
	ResponseModel    string
	ResponseID       string
	Timestamp        time.Time
}

// Compact summarizes old complete turns and appends a durable compaction
// boundary. The original entries remain in the session log. It is deliberately
// manual in v1; automatic threshold and overflow retry policies can call this
// same operation later.
func (s *Session) Compact(ctx context.Context, instructions string) (CompactionResult, error) {
	if !s.runMu.TryLock() {
		return CompactionResult{}, ErrBusy
	}
	defer s.runMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	state := s.agent.Snapshot()
	if state.IsStreaming {
		return CompactionResult{}, ErrBusy
	}
	// A prior run may have reached the model but failed while persisting its
	// messages. Compact must not project from an older durable prefix and thereby
	// discard those in-memory messages.
	if err := s.persistNew(ctx); err != nil {
		return CompactionResult{}, err
	}
	entries, leafID := s.snapshotTranscript()
	keepRecent := defaultKeepRecentTokens
	if state.Model.ContextWindow > 0 && state.Model.ContextWindow/4 < keepRecent {
		keepRecent = state.Model.ContextWindow / 4
	}
	if keepRecent <= 0 {
		keepRecent = defaultKeepRecentTokens
	}
	prepared, err := compaction.Prepare(entries, leafID, keepRecent)
	if err != nil {
		return CompactionResult{}, err
	}
	response, err := s.compactor.Compact(ctx, compaction.Request{
		Model:           state.Model,
		Messages:        prepared.Messages,
		PreviousSummary: prepared.PreviousSummary,
		Instructions:    instructions,
	})
	if err != nil {
		return CompactionResult{}, err
	}
	summary := strings.TrimSpace(response.Summary)
	if summary == "" {
		return CompactionResult{}, errors.New("coding: compactor returned an empty summary")
	}
	if err := ctx.Err(); err != nil {
		return CompactionResult{}, err
	}

	entry := transcript.NewCompaction(leafID, transcript.Compaction{
		Summary:           summary,
		FirstKeptEntryID:  prepared.FirstKeptID,
		TokensBefore:      prepared.TokensBefore,
		Provider:          response.Provider,
		Model:             response.Model,
		ResponseModel:     response.ResponseModel,
		ResponseID:        response.ResponseID,
		Usage:             response.Usage,
		ResponseTimestamp: response.Timestamp,
	})
	candidate := append(append([]transcript.Entry(nil), entries...), entry)
	projected, err := transcript.BuildContext(candidate, entry.ID)
	if err != nil {
		return CompactionResult{}, err
	}
	tokensAfter := compaction.EstimateMessages(projected)
	entry.Compaction.TokensAfter = tokensAfter
	candidate[len(candidate)-1] = entry

	if s.store != nil {
		if err := s.store.Append(ctx, entry); err != nil {
			return CompactionResult{}, err
		}
	}
	// Persistence is the commit point. Nothing observable changes before it.
	s.agent.SetMessages(projected)
	s.transcriptMu.Lock()
	s.entries = candidate
	s.leafID = entry.ID
	s.usageStart = len(projected)
	s.persistedLen = len(projected)
	s.transcriptMu.Unlock()
	s.dispatchEvent(Event{
		Type: CompactionCompleted, Usage: response.Usage,
		Provider: response.Provider, Model: response.Model,
		ResponseModel: response.ResponseModel, ResponseID: response.ResponseID,
		Timestamp: response.Timestamp,
	})

	return CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: prepared.FirstKeptID,
		TokensBefore:     prepared.TokensBefore,
		TokensAfter:      tokensAfter,
		Usage:            response.Usage,
		Provider:         response.Provider,
		Model:            response.Model,
		ResponseModel:    response.ResponseModel,
		ResponseID:       response.ResponseID,
		Timestamp:        response.Timestamp,
	}, nil
}

// Keep errors.Is useful for product adapters without importing the compaction
// implementation package.
func IsNothingToCompact(err error) bool {
	return errors.Is(err, compaction.ErrNothingToCompact)
}
