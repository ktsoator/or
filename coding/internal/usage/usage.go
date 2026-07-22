// Package usage is the coding product's token and cost ledger. It is kept
// separate from any delivery mechanism: the ledger is written from the session
// event stream and read by whichever client asks for a report, so nothing
// here depends on HTTP.
package usage

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	Requests    int64         `json:"requests"`
	Input       int64         `json:"input"`
	Output      int64         `json:"output"`
	CacheRead   int64         `json:"cacheRead"`
	CacheWrite  int64         `json:"cacheWrite"`
	TotalTokens int64         `json:"totalTokens"`
	Cost        llm.UsageCost `json:"cost"`
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

// Store is an append-only, deduplicated usage ledger.
type Store struct {
	path string

	mu     sync.Mutex
	events map[string]Event
}

func NewStore(path string) (*Store, error) {
	store := &Store{path: path, events: make(map[string]Event)}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usage: open ledger: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("usage: decode ledger: %w", err)
		}
		if event.ID != "" {
			store.events[event.ID] = event
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("usage: read ledger: %w", err)
	}
	return store, nil
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

// Backfill adds provider responses already present in a restored transcript.
// Stable event IDs make the operation safe to run on every startup.
func (s *Store) Backfill(sessionID string, messages []agent.AgentMessage) error {
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			continue
		}
		assistant, ok := llmMessage.(*llm.AssistantMessage)
		if !ok || assistant == nil || !present(assistant.Usage) {
			continue
		}
		if err := s.appendAssistant(sessionID, assistant); err != nil {
			return err
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

func (s *Store) append(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[event.ID]; exists {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("usage: create ledger directory: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("usage: open ledger for append: %w", err)
	}
	data, err := json.Marshal(event)
	if err == nil {
		data = append(data, '\n')
		_, err = file.Write(data)
	}
	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("usage: append ledger: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("usage: close ledger: %w", closeErr)
	}
	s.events[event.ID] = event
	return nil
}

// Report aggregates a stable snapshot without holding the lock while sorting.
// A zero since value includes the complete ledger.
func (s *Store) Report(since time.Time) Report {
	s.mu.Lock()
	events := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		events = append(events, event)
	}
	s.mu.Unlock()

	groups := make(map[string]*ModelSummary)
	report := Report{GeneratedAt: time.Now().UTC()}
	for _, event := range events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		addTotals(&report.Total, event.Usage)
		key := event.Provider + "\x00" + event.Model
		group := groups[key]
		if group == nil {
			name := event.Model
			if model, ok := llm.LookupModel(event.Provider, event.Model); ok && model.Name != "" {
				name = model.Name
			}
			group = &ModelSummary{
				Provider: event.Provider,
				Model:    event.Model,
				Name:     name,
			}
			groups[key] = group
		}
		addTotals(&group.Totals, event.Usage)
		if event.Timestamp.After(group.LastUsedAt) {
			group.LastUsedAt = event.Timestamp
			group.ResponseModel = event.ResponseModel
		}
	}
	for _, group := range groups {
		report.Models = append(report.Models, *group)
	}
	sort.Slice(report.Models, func(i, j int) bool {
		if report.Models[i].TotalTokens == report.Models[j].TotalTokens {
			return report.Models[i].LastUsedAt.After(report.Models[j].LastUsedAt)
		}
		return report.Models[i].TotalTokens > report.Models[j].TotalTokens
	})
	return report
}

// Events returns individual requests filtered by provider and model. Results
// are newest first and paginated so the usage page stays fast as the ledger
// grows.
func (s *Store) Events(provider, model string, since time.Time, offset, limit int) EventPage {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	s.mu.Lock()
	events := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		if provider != "" && event.Provider != provider {
			continue
		}
		if model != "" && event.Model != model {
			continue
		}
		events = append(events, event)
	}
	s.mu.Unlock()
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].ID > events[j].ID
		}
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	total := len(events)
	if offset >= total {
		return EventPage{Events: []Event{}, Total: total, Limit: limit, Offset: offset}
	}
	end := min(offset+limit, total)
	return EventPage{
		Events: events[offset:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

func addTotals(total *Totals, usage llm.Usage) {
	total.Requests++
	total.Input += usage.Input
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
	return usage.Input != 0 || usage.Output != 0 || usage.CacheRead != 0 ||
		usage.CacheWrite != 0 || usage.TotalTokens != 0 || usage.Cost.Total != 0
}

func eventID(sessionID, provider, model, responseID string, timestamp time.Time, usage llm.Usage) string {
	if responseID != "" {
		return provider + ":" + responseID
	}
	payload := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d",
		sessionID, provider, model, timestamp.UnixMilli(),
		usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite)
	sum := sha256.Sum256([]byte(payload))
	return "local:" + hex.EncodeToString(sum[:])
}

func normalizedTime(timestamp time.Time) time.Time {
	if timestamp.IsZero() || timestamp.UnixMilli() <= 0 {
		return time.Now().UTC()
	}
	return timestamp.UTC()
}
