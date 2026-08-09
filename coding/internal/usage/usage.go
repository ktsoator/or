// Package usage is the coding product's token and cost ledger. It is kept
// separate from any delivery mechanism: the ledger is written from the session
// event stream and read by whichever client asks for a report, so nothing
// here depends on HTTP.
package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// Event is one billable provider response. It is stored independently
// from conversations so deleting a session does not rewrite usage history.
type Event struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"sessionId"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	ResponseModel string    `json:"responseModel,omitempty"`
	ResponseID    string    `json:"responseId,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Usage         llm.Usage `json:"usage"`
}

// Totals is an aggregate returned by the usage API.
type Totals struct {
	Requests     int64         `json:"requests"`
	Input        int64         `json:"input"`
	InputUnknown bool          `json:"inputUnknown,omitempty"`
	Output       int64         `json:"output"`
	CacheRead    int64         `json:"cacheRead"`
	CacheWrite   int64         `json:"cacheWrite"`
	TotalTokens  int64         `json:"totalTokens"`
	Cost         llm.UsageCost `json:"cost"`
}

// ModelSummary groups usage by the requested provider and model.
type ModelSummary struct {
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Name          string    `json:"name"`
	ResponseModel string    `json:"responseModel,omitempty"`
	LastUsedAt    time.Time `json:"lastUsedAt"`
	Totals
}

// Report is an aggregate over a requested time range.
type Report struct {
	Total       Totals         `json:"total"`
	Models      []ModelSummary `json:"models"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

// EventPage is a newest-first slice of individual provider requests.
type EventPage struct {
	Events []Event `json:"events"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// RecordEvent persists one live MessageCompleted event. Empty usage records
// are ignored because some providers emit terminal metadata without billing.
func (s *Store) RecordEvent(sessionID string, event engine.Event) error {
	if (event.Type != engine.MessageCompleted && event.Type != engine.CompactionCompleted) || !present(event.Usage) {
		return nil
	}
	return s.append(Event{
		ID:            eventID(sessionID, event.Provider, event.Model, event.ResponseID, event.Timestamp, event.Usage),
		SessionID:     sessionID,
		Provider:      event.Provider,
		Model:         event.Model,
		ResponseModel: event.ResponseModel,
		ResponseID:    event.ResponseID,
		Timestamp:     normalizedTime(event.Timestamp),
		Usage:         event.Usage,
	})
}

// BackfillEntries restores usage for both ordinary assistant responses and the
// direct model requests used to create compaction checkpoints.
func (s *Store) BackfillEntries(sessionID string, entries []transcript.Entry) error {
	for _, entry := range entries {
		switch entry.Type {
		case transcript.MessageEntry:
			message, ok := agent.ToLLM(entry.Message)
			if !ok {
				continue
			}
			assistant, ok := message.(*llm.AssistantMessage)
			if !ok || assistant == nil || !present(assistant.Usage) {
				continue
			}
			if err := s.appendAssistant(sessionID, assistant); err != nil {
				return err
			}
		case transcript.CompactionEntry:
			compact := entry.Compaction
			if compact == nil || !present(compact.Usage) {
				continue
			}
			timestamp := compact.ResponseTimestamp
			if timestamp.IsZero() {
				timestamp = entry.Timestamp
			}
			if err := s.append(Event{
				ID:        eventID(sessionID, compact.Provider, compact.Model, compact.ResponseID, timestamp, compact.Usage),
				SessionID: sessionID, Provider: compact.Provider, Model: compact.Model,
				ResponseModel: compact.ResponseModel, ResponseID: compact.ResponseID,
				Timestamp: normalizedTime(timestamp), Usage: compact.Usage,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) appendAssistant(sessionID string, assistant *llm.AssistantMessage) error {
	timestamp := time.UnixMilli(assistant.Timestamp).UTC()
	return s.append(Event{
		ID:            eventID(sessionID, assistant.Provider, assistant.Model, assistant.ResponseID, timestamp, assistant.Usage),
		SessionID:     sessionID,
		Provider:      assistant.Provider,
		Model:         assistant.Model,
		ResponseModel: assistant.ResponseModel,
		ResponseID:    assistant.ResponseID,
		Timestamp:     normalizedTime(timestamp),
		Usage:         assistant.Usage,
	})
}

func addTotals(total *Totals, usage llm.Usage) {
	total.Requests++
	total.Input += usage.Input
	total.InputUnknown = total.InputUnknown || usage.InputUnknown
	total.Output += usage.Output
	total.CacheRead += usage.CacheRead
	total.CacheWrite += usage.CacheWrite
	tokens := usage.TotalTokens
	if tokens == 0 {
		tokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	}
	total.TotalTokens += tokens
	total.Cost.Input += usage.Cost.Input
	total.Cost.Output += usage.Cost.Output
	total.Cost.CacheRead += usage.Cost.CacheRead
	total.Cost.CacheWrite += usage.Cost.CacheWrite
	total.Cost.Total += usage.Cost.Total
}

func present(usage llm.Usage) bool {
	return usage.InputUnknown || usage.Input != 0 || usage.Output != 0 || usage.CacheRead != 0 ||
		usage.CacheWrite != 0 || usage.TotalTokens != 0 || usage.Cost.Total != 0
}

func eventID(sessionID, provider, model, responseID string, timestamp time.Time, usage llm.Usage) string {
	if responseID != "" {
		return provider + ":" + responseID
	}
	payload := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00%t\x00%d\x00%d\x00%d",
		sessionID, provider, model, timestamp.UnixMilli(),
		usage.Input, usage.InputUnknown, usage.Output, usage.CacheRead, usage.CacheWrite)
	sum := sha256.Sum256([]byte(payload))
	return "local:" + hex.EncodeToString(sum[:])
}

func normalizedTime(timestamp time.Time) time.Time {
	if timestamp.IsZero() || timestamp.UnixMilli() <= 0 {
		return time.Now().UTC()
	}
	return timestamp.UTC()
}
