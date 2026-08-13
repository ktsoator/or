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

func TestJSONLRecorderWritesProviderPerformanceSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.jsonl")
	recorder, err := NewJSONL(path, FileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	recorder.Record(Event{
		Name: ProviderCompleted, Timestamp: startedAt.Add(2500 * time.Millisecond),
		SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1",
		Status: "completed", StartedAt: startedAt, Duration: 2500 * time.Millisecond,
		TimeToFirstOutput: 1250 * time.Millisecond,
		Provider:          "provider-1", Model: "model-1", ResponseModel: "model-1-2026-08",
		ProviderResponseID: "response-1", StopReason: "stop",
		Attempt: 2, HTTPStatus: 200, MessageCount: 4, AttachmentCount: 1,
		InputTokens: 11, InputUnknown: true, OutputTokens: 7,
		CacheReadTokens: 3, CacheWriteTokens: 2, TotalTokens: 23,
		CostInput: 0.01, CostOutput: 0.14, CostCacheRead: 0.01,
		CostCacheWrite: 0.04, CostTotal: 0.20,
	})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	records := readJSONLRecords(t, path)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	want := map[string]any{
		"event": ProviderCompleted, "session_id": "session-1", "run_id": "run-1",
		"turn_id": "turn-1", "provider_request_id": "request-1", "status": "completed",
		"duration_ms": float64(2500), "time_to_first_output_ms": float64(1250),
		"provider": "provider-1", "model": "model-1", "response_model": "model-1-2026-08",
		"provider_response_id": "response-1", "stop_reason": "stop",
		"attempt": float64(2), "http_status": float64(200),
		"message_count": float64(4), "attachment_count": float64(1),
		"input_tokens": float64(11), "input_unknown": true, "output_tokens": float64(7),
		"cache_read_tokens": float64(3), "cache_write_tokens": float64(2),
		"total_tokens": float64(23), "cost_input_usd": 0.01, "cost_output_usd": 0.14,
		"cost_cache_read_usd": 0.01, "cost_cache_write_usd": 0.04, "cost_total_usd": 0.20,
	}
	for key, wantValue := range want {
		if got := record[key]; got != wantValue {
			t.Fatalf("record[%q] = %#v, want %#v; record = %#v", key, got, wantValue, record)
		}
	}
	for _, forbidden := range []string{"url", "body", "headers", "error", "prompt"} {
		if _, found := record[forbidden]; found {
			t.Fatalf("record contains forbidden field %q: %#v", forbidden, record)
		}
	}
}

func TestJSONLRecorderWritesToolAndApprovalSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.jsonl")
	recorder, err := NewJSONL(path, FileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	for _, event := range []Event{
		{
			Name: ToolCompleted, Timestamp: startedAt.Add(1250 * time.Millisecond),
			SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1",
			ToolCallID: "call-1", ToolName: "shell", Status: "success",
			StartedAt: startedAt, Duration: 1250 * time.Millisecond,
		},
		{
			Name: ApprovalCompleted, Timestamp: startedAt.Add(750 * time.Millisecond),
			SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1",
			ToolCallID: "call-1", ToolName: "shell", Status: "allowed",
			StartedAt: startedAt, Duration: 750 * time.Millisecond,
		},
	} {
		recorder.Record(event)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	records := readJSONLRecords(t, path)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	for index, record := range records {
		if record["session_id"] != "session-1" || record["run_id"] != "run-1" ||
			record["turn_id"] != "turn-1" || record["provider_request_id"] != "request-1" ||
			record["tool_call_id"] != "call-1" || record["tool_name"] != "shell" {
			t.Fatalf("record %d correlation = %#v", index, record)
		}
	}
	if records[0]["event"] != ToolCompleted || records[0]["status"] != "success" ||
		records[0]["duration_ms"] != float64(1250) {
		t.Fatalf("tool record = %#v", records[0])
	}
	if records[1]["event"] != ApprovalCompleted || records[1]["status"] != "allowed" ||
		records[1]["duration_ms"] != float64(750) {
		t.Fatalf("approval record = %#v", records[1])
	}
	for _, record := range records {
		for _, forbidden := range []string{"arguments", "result", "path", "reason", "error"} {
			if _, found := record[forbidden]; found {
				t.Fatalf("record contains forbidden field %q: %#v", forbidden, record)
			}
		}
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

func readJSONLRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
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
	return records
}
