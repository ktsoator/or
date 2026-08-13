package observability

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONLRecorderWritesPrivateStructuredEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "observability.jsonl")
	recorder, err := NewJSONL(path, FileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	recorder.Record(Event{
		Name: ApplicationStarted,
	})
	recorder.Record(Event{
		Name:      RunCompleted,
		Timestamp: startedAt.Add(1500 * time.Millisecond),
		SessionID: "session-1",
		RunID:     "run-1",
		Status:    "completed",
		StartedAt: startedAt,
		Duration:  1500 * time.Millisecond,
	})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log mode = %o, want 600", got)
	}
	if dirInfo, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("log directory mode = %o, want 700", got)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var records []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	completed := records[1]
	if completed["event"] != RunCompleted || completed["session_id"] != "session-1" ||
		completed["run_id"] != "run-1" || completed["duration_ms"] != float64(1500) {
		t.Fatalf("completed record = %#v", completed)
	}
	if _, found := completed["msg"]; found {
		t.Fatalf("completed record retained slog msg field: %#v", completed)
	}
}

func TestJSONLRecorderRotatesBoundedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.jsonl")
	recorder, err := NewJSONL(path, FileOptions{MaxBytes: 220, MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 12; index++ {
		recorder.Record(Event{
			Name:      RunFailed,
			SessionID: strings.Repeat("s", 32),
			RunID:     NewID("run"),
			Status:    "failed",
			ErrorCode: "run_failed",
		})
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(candidate); err != nil {
			t.Fatalf("expected rotated file %s: %v", candidate, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatalf("unexpected fourth log file: %v", err)
	}
}

func TestNewIDUsesRequestedPrefix(t *testing.T) {
	first := NewID("run")
	second := NewID("run")
	if !strings.HasPrefix(first, "run_") || first == second {
		t.Fatalf("ids = %q, %q", first, second)
	}
}
