package web

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
	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/llm"
)

// UsageEvent is one billable provider response. It is stored independently
// from conversations so deleting a session does not rewrite usage history.
type UsageEvent struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"sessionId"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	ResponseModel string    `json:"responseModel,omitempty"`
	ResponseID    string    `json:"responseId,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Usage         llm.Usage `json:"usage"`
}

// UsageTotals is an aggregate returned by the usage API.
type UsageTotals struct {
	Requests    int64         `json:"requests"`
	Input       int64         `json:"input"`
	Output      int64         `json:"output"`
	CacheRead   int64         `json:"cacheRead"`
	CacheWrite  int64         `json:"cacheWrite"`
	TotalTokens int64         `json:"totalTokens"`
	Cost        llm.UsageCost `json:"cost"`
}

// ModelUsageSummary groups usage by the requested provider and model.
type ModelUsageSummary struct {
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Name          string    `json:"name"`
	ResponseModel string    `json:"responseModel,omitempty"`
	LastUsedAt    time.Time `json:"lastUsedAt"`
	UsageTotals
}

// UsageReport is an aggregate over a requested time range.
type UsageReport struct {
	Total       UsageTotals         `json:"total"`
	Models      []ModelUsageSummary `json:"models"`
	GeneratedAt time.Time           `json:"generatedAt"`
}

// UsageEventPage is a newest-first slice of individual provider requests.
type UsageEventPage struct {
	Events []UsageEvent `json:"events"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// UsageStore is an append-only, deduplicated usage ledger.
type UsageStore struct {
	path string

	mu     sync.Mutex
	events map[string]UsageEvent
}

func NewUsageStore(path string) (*UsageStore, error) {
	store := &UsageStore{path: path, events: make(map[string]UsageEvent)}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("web: open usage ledger: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var event UsageEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("web: decode usage ledger: %w", err)
		}
		if event.ID != "" {
			store.events[event.ID] = event
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("web: read usage ledger: %w", err)
	}
	return store, nil
}

// RecordEvent persists one live MessageCompleted event. Empty usage records
// are ignored because some providers emit terminal metadata without billing.
func (s *UsageStore) RecordEvent(sessionID string, event coding.Event) error {
	if event.Type != coding.MessageCompleted || !usagePresent(event.Usage) {
		return nil
	}
	return s.append(UsageEvent{
		ID:            usageEventID(sessionID, event.Provider, event.Model, event.ResponseID, event.Timestamp, event.Usage),
		SessionID:     sessionID,
		Provider:      event.Provider,
		Model:         event.Model,
		ResponseModel: event.ResponseModel,
		ResponseID:    event.ResponseID,
		Timestamp:     normalizedUsageTime(event.Timestamp),
		Usage:         event.Usage,
	})
}

// Backfill adds provider responses already present in a restored transcript.
// Stable event IDs make the operation safe to run on every startup.
func (s *UsageStore) Backfill(sessionID string, messages []agent.AgentMessage) error {
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			continue
		}
		assistant, ok := llmMessage.(*llm.AssistantMessage)
		if !ok || assistant == nil || !usagePresent(assistant.Usage) {
			continue
		}
		timestamp := time.UnixMilli(assistant.Timestamp).UTC()
		if err := s.append(UsageEvent{
			ID:            usageEventID(sessionID, assistant.Provider, assistant.Model, assistant.ResponseID, timestamp, assistant.Usage),
			SessionID:     sessionID,
			Provider:      assistant.Provider,
			Model:         assistant.Model,
			ResponseModel: assistant.ResponseModel,
			ResponseID:    assistant.ResponseID,
			Timestamp:     normalizedUsageTime(timestamp),
			Usage:         assistant.Usage,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *UsageStore) append(event UsageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[event.ID]; exists {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("web: create usage directory: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("web: open usage ledger for append: %w", err)
	}
	data, err := json.Marshal(event)
	if err == nil {
		data = append(data, '\n')
		_, err = file.Write(data)
	}
	closeErr := file.Close()
	if err != nil {
		return fmt.Errorf("web: append usage ledger: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("web: close usage ledger: %w", closeErr)
	}
	s.events[event.ID] = event
	return nil
}

// Report aggregates a stable snapshot without holding the lock while sorting.
// A zero since value includes the complete ledger.
func (s *UsageStore) Report(since time.Time) UsageReport {
	s.mu.Lock()
	events := make([]UsageEvent, 0, len(s.events))
	for _, event := range s.events {
		events = append(events, event)
	}
	s.mu.Unlock()

	groups := make(map[string]*ModelUsageSummary)
	report := UsageReport{GeneratedAt: time.Now().UTC()}
	for _, event := range events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		addUsageTotals(&report.Total, event.Usage)
		key := event.Provider + "\x00" + event.Model
		group := groups[key]
		if group == nil {
			name := event.Model
			if model, ok := llm.LookupModel(event.Provider, event.Model); ok && model.Name != "" {
				name = model.Name
			}
			group = &ModelUsageSummary{
				Provider: event.Provider,
				Model:    event.Model,
				Name:     name,
			}
			groups[key] = group
		}
		addUsageTotals(&group.UsageTotals, event.Usage)
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
func (s *UsageStore) Events(provider, model string, since time.Time, offset, limit int) UsageEventPage {
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
	events := make([]UsageEvent, 0, len(s.events))
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
		return UsageEventPage{Events: []UsageEvent{}, Total: total, Limit: limit, Offset: offset}
	}
	end := min(offset+limit, total)
	return UsageEventPage{
		Events: events[offset:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
}

func addUsageTotals(total *UsageTotals, usage llm.Usage) {
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

func usagePresent(usage llm.Usage) bool {
	return usage.Input != 0 || usage.Output != 0 || usage.CacheRead != 0 ||
		usage.CacheWrite != 0 || usage.TotalTokens != 0 || usage.Cost.Total != 0
}

func usageEventID(sessionID, provider, model, responseID string, timestamp time.Time, usage llm.Usage) string {
	if responseID != "" {
		return provider + ":" + responseID
	}
	payload := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d",
		sessionID, provider, model, timestamp.UnixMilli(),
		usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite)
	sum := sha256.Sum256([]byte(payload))
	return "local:" + hex.EncodeToString(sum[:])
}

func normalizedUsageTime(timestamp time.Time) time.Time {
	if timestamp.IsZero() || timestamp.UnixMilli() <= 0 {
		return time.Now().UTC()
	}
	return timestamp.UTC()
}
