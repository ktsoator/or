package usage

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/llm"
)

func TestAddTotalsPreservesUnknownInput(t *testing.T) {
	var total Totals
	addTotals(&total, llm.Usage{InputUnknown: true, Output: 5, TotalTokens: 5})
	addTotals(&total, llm.Usage{Input: 3, Output: 2, TotalTokens: 5})

	if total.Requests != 2 || !total.InputUnknown || total.Input != 3 || total.Output != 7 || total.TotalTokens != 10 {
		t.Fatalf("totals = %#v, want known input retained and aggregate marked unknown", total)
	}
}

func TestStoreDeduplicatesReportsAndPaginates(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "usage", "events.jsonl")
	store, err := NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, time.July, 30, 10, 0, 0, 123, time.UTC)
	events := []engine.Event{
		{
			Type:          engine.MessageCompleted,
			Provider:      "provider-a",
			Model:         "model-a",
			ResponseModel: "model-a-2026-01",
			ResponseID:    "response-1",
			Timestamp:     base,
			Usage: llm.Usage{
				Input: 4, Output: 6, TotalTokens: 10,
				Cost: llm.UsageCost{Total: 0.10},
			},
		},
		{
			Type:          engine.MessageCompleted,
			Provider:      "provider-a",
			Model:         "model-a",
			ResponseModel: "model-a-2026-02",
			ResponseID:    "response-2",
			Timestamp:     base.Add(time.Hour),
			Usage: llm.Usage{
				InputUnknown: true, Output: 5, TotalTokens: 5,
				Cost: llm.UsageCost{Total: 0.05},
			},
		},
		{
			Type:          engine.CompactionCompleted,
			Provider:      "provider-b",
			Model:         "model-b",
			ResponseModel: "model-b-2026-01",
			ResponseID:    "response-3",
			Timestamp:     base.Add(2 * time.Hour),
			Usage: llm.Usage{
				Input: 12, Output: 8, TotalTokens: 20,
				Cost: llm.UsageCost{Total: 0.20},
			},
		},
	}
	for index := len(events) - 1; index >= 0; index-- {
		if err := store.RecordEvent("session-1", events[index]); err != nil {
			t.Fatal(err)
		}
	}
	duplicate := events[1]
	duplicate.Usage = llm.Usage{Output: 999, TotalTokens: 999}
	if err := store.RecordEvent("session-2", duplicate); err != nil {
		t.Fatal(err)
	}

	report, err := store.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 3 ||
		report.Total.TotalTokens != 35 ||
		!report.Total.InputUnknown ||
		math.Abs(report.Total.Cost.Total-0.35) > 1e-12 {
		t.Fatalf("all-time report = %+v", report)
	}
	if len(report.Models) != 2 ||
		report.Models[0].Model != "model-b" ||
		report.Models[1].Model != "model-a" ||
		report.Models[1].ResponseModel != "model-a-2026-02" {
		t.Fatalf("model summaries = %+v", report.Models)
	}

	sinceReport, err := store.Report(base.Add(30 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if sinceReport.Total.Requests != 2 || sinceReport.Total.TotalTokens != 25 {
		t.Fatalf("filtered report = %+v", sinceReport)
	}

	first, err := store.Events("provider-a", "model-a", time.Time{}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 2 ||
		len(first.Events) != 1 ||
		first.Events[0].ResponseID != "response-2" ||
		!first.Events[0].Usage.InputUnknown {
		t.Fatalf("first event page = %+v", first)
	}
	second, err := store.Events("provider-a", "model-a", time.Time{}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 2 || len(second.Events) != 1 || second.Events[0].ResponseID != "response-1" {
		t.Fatalf("second event page = %+v", second)
	}
	modelOnly, err := store.Events("", "model-b", base.Add(time.Hour), 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if modelOnly.Total != 1 || len(modelOnly.Events) != 1 || modelOnly.Events[0].Model != "model-b" {
		t.Fatalf("model-filtered events = %+v", modelOnly)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reopened, err := store.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Total.Requests != 3 || reopened.Total.TotalTokens != 35 {
		t.Fatalf("reopened report = %+v", reopened)
	}
	info, err := os.Stat(databasePath(legacyPath))
	if err != nil {
		t.Fatalf("SQLite ledger was not created: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("SQLite ledger permissions = %o, want 600", got)
	}
}

func TestStoreMigratesLegacyJSONLIncrementally(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "usage", "events.jsonl")
	first := Event{
		ID:            "legacy-1",
		SessionID:     "session-1",
		Provider:      "provider-a",
		Model:         "model-a",
		ResponseModel: "model-a-old",
		ResponseID:    "response-1",
		Timestamp:     time.Date(2026, time.July, 1, 10, 0, 0, 0, time.UTC),
		Usage:         llm.Usage{Input: 3, Output: 2, TotalTokens: 5},
	}
	replacement := first
	replacement.ResponseModel = "model-a-replaced"
	replacement.Usage = llm.Usage{Input: 4, Output: 3, TotalTokens: 7}
	writeLegacyEvents(t, legacyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, first, replacement)

	store, err := NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 1 ||
		report.Total.TotalTokens != 7 ||
		len(report.Models) != 1 ||
		report.Models[0].ResponseModel != "model-a-replaced" {
		t.Fatalf("initial migrated report = %+v", report)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	duplicate := replacement
	duplicate.Usage = llm.Usage{TotalTokens: 999}
	second := Event{
		ID:            "legacy-2",
		SessionID:     "session-2",
		Provider:      "provider-b",
		Model:         "model-b",
		ResponseModel: "model-b-new",
		ResponseID:    "response-2",
		Timestamp:     first.Timestamp.Add(time.Hour),
		Usage:         llm.Usage{Input: 6, Output: 7, TotalTokens: 13},
	}
	writeLegacyEvents(t, legacyPath, os.O_APPEND|os.O_WRONLY, duplicate, second)

	store, err = NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	report, err = store.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 2 || report.Total.TotalTokens != 20 {
		t.Fatalf("incrementally migrated report = %+v", report)
	}
	page, err := store.Events("", "", time.Time{}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Events) != 2 || page.Events[0].ID != second.ID {
		t.Fatalf("migrated events = %+v", page)
	}
}

func TestStoreRollsBackInvalidLegacyMigration(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "usage", "events.jsonl")
	event := Event{
		ID:        "legacy-1",
		SessionID: "session-1",
		Provider:  "provider-a",
		Model:     "model-a",
		Timestamp: time.Date(2026, time.July, 1, 10, 0, 0, 0, time.UTC),
		Usage:     llm.Usage{Input: 3, Output: 2, TotalTokens: 5},
	}
	writeLegacyEvents(t, legacyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, event)
	file, err := os.OpenFile(legacyPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("not-json\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := NewStore(legacyPath); err == nil {
		_ = store.Close()
		t.Fatal("NewStore succeeded with an invalid legacy ledger")
	}

	writeLegacyEvents(t, legacyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, event)
	store, err := NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	report, err := store.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 1 || report.Total.TotalTokens != 5 {
		t.Fatalf("report after migration retry = %+v", report)
	}
}

func TestStoreIndexesTimestampsOutsideUnixNanoRange(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "usage", "events.jsonl")
	early := Event{
		ID:        "early",
		SessionID: "session-1",
		Provider:  "provider-a",
		Model:     "model-a",
		Timestamp: time.Date(1600, time.January, 1, 0, 0, 0, 1, time.UTC),
		Usage:     llm.Usage{TotalTokens: 3},
	}
	late := Event{
		ID:        "late",
		SessionID: "session-2",
		Provider:  "provider-a",
		Model:     "model-a",
		Timestamp: time.Date(2500, time.January, 1, 0, 0, 0, 2, time.UTC),
		Usage:     llm.Usage{TotalTokens: 5},
	}
	writeLegacyEvents(t, legacyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, early, late)
	store, err := NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	page, err := store.Events("", "", time.Time{}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 ||
		len(page.Events) != 2 ||
		page.Events[0].ID != late.ID ||
		page.Events[1].ID != early.ID {
		t.Fatalf("wide-range events = %+v", page)
	}
	report, err := store.Report(time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 1 || report.Total.TotalTokens != 5 {
		t.Fatalf("wide-range report = %+v", report)
	}
}

func TestStoreDoesNotOverwriteDatabaseWhenLegacyAppearsLater(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "usage", "events.jsonl")
	store, err := NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	event := engine.Event{
		Type:          engine.MessageCompleted,
		Provider:      "provider-a",
		Model:         "model-a",
		ResponseModel: "sqlite-value",
		ResponseID:    "response-1",
		Timestamp:     time.Date(2026, time.July, 1, 10, 0, 0, 0, time.UTC),
		Usage:         llm.Usage{TotalTokens: 5},
	}
	if err := store.RecordEvent("session-1", event); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	legacy := Event{
		ID:            "provider-a:response-1",
		SessionID:     "session-1",
		Provider:      event.Provider,
		Model:         event.Model,
		ResponseModel: "legacy-value",
		ResponseID:    event.ResponseID,
		Timestamp:     event.Timestamp,
		Usage:         llm.Usage{TotalTokens: 999},
	}
	writeLegacyEvents(t, legacyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, legacy)

	store, err = NewStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err := store.Events("", "", time.Time{}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 ||
		len(page.Events) != 1 ||
		page.Events[0].ResponseModel != "sqlite-value" ||
		page.Events[0].Usage.TotalTokens != 5 {
		t.Fatalf("database event was overwritten by legacy data: %+v", page)
	}
}

func writeLegacyEvents(t *testing.T, path string, flags int, events ...Event) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
