package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadDiagnosticReportAggregatesAndFiltersRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.jsonl")
	recorder, err := NewJSONL(path, FileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	for _, event := range []Event{
		{Name: RunStarted, Timestamp: startedAt, SessionID: "session-1", RunID: "run-1", Status: "running", StartedAt: startedAt},
		{Name: ProviderStarted, Timestamp: startedAt.Add(time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", Status: "running"},
		{Name: CheckpointCompleted, Timestamp: startedAt.Add(2 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", Status: "completed", Duration: 12 * time.Millisecond},
		{Name: ApprovalStarted, Timestamp: startedAt.Add(3 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", ToolCallID: "call-1", ToolName: "shell", Status: "waiting"},
		{Name: ApprovalCompleted, Timestamp: startedAt.Add(4 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", ToolCallID: "call-1", ToolName: "shell", Status: "allowed", Duration: 20 * time.Millisecond},
		{Name: ToolStarted, Timestamp: startedAt.Add(5 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", ToolCallID: "call-1", ToolName: "shell", Status: "running"},
		{Name: ToolCompleted, Timestamp: startedAt.Add(6 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", ToolCallID: "call-1", ToolName: "shell", Status: "success", Duration: 45 * time.Millisecond},
		{Name: TurnDiscarded, Timestamp: startedAt.Add(7 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", Status: "discarded", Reason: "retry"},
		{Name: ProviderCompleted, Timestamp: startedAt.Add(8 * time.Millisecond), SessionID: "session-1", RunID: "run-1", TurnID: "turn-1", RequestID: "request-1", Status: "completed", Duration: 80 * time.Millisecond, TimeToFirstOutput: 25 * time.Millisecond, InputTokens: 10, OutputTokens: 5, TotalTokens: 15, CostTotal: 0.12},
		{Name: RunCompleted, Timestamp: startedAt.Add(100 * time.Millisecond), SessionID: "session-1", RunID: "run-1", Status: "completed", StartedAt: startedAt, Duration: 100 * time.Millisecond},
		{Name: RunCompleted, Timestamp: startedAt.Add(time.Second), SessionID: "session-2", RunID: "run-2", Status: "completed", StartedAt: startedAt, Duration: time.Second},
	} {
		recorder.Record(event)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := ReadDiagnosticReport(path, DiagnosticQuery{SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("runs = %#v, want one", report.Runs)
	}
	run := report.Runs[0]
	if run.ID != "run-1" || run.Status != "completed" || run.DurationMS != 100 ||
		run.TimeToFirstOutputMS != 25 || run.CheckpointDurationMS != 12 ||
		run.ToolDurationMS != 45 || run.ApprovalDurationMS != 20 ||
		run.ProviderRequests != 1 || run.ToolCalls != 1 || run.ApprovalRequests != 1 ||
		run.Retries != 1 || run.TotalTokens != 15 || run.CostTotalUSD != 0.12 {
		t.Fatalf("run = %#v", run)
	}
	if len(run.Events) != 10 || run.Events[3].ToolCallID != "call-1" || run.Events[3].ToolName != "shell" {
		t.Fatalf("events = %#v", run.Events)
	}
}

func TestReadDiagnosticReportIncludesRotationsAndIgnoresPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observability.jsonl")
	if err := os.WriteFile(path+".1", []byte("{\"time\":\"2026-08-13T12:00:00Z\",\"event\":\"run.completed\",\"session_id\":\"session-1\",\"run_id\":\"old\",\"status\":\"completed\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"time\":\"2026-08-13T13:00:00Z\",\"event\":\"run.started\",\"session_id\":\"session-1\",\"run_id\":\"new\",\"status\":\"running\"}\n{\"time\":"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ReadDiagnosticReport(path, DiagnosticQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 2 || report.Runs[0].ID != "new" || report.Runs[1].ID != "old" {
		t.Fatalf("runs = %#v", report.Runs)
	}
}

func TestReadDiagnosticReportDropsUnapprovedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.jsonl")
	const sensitive = "do-not-return-this-value"
	line := `{"time":"2026-08-13T12:00:00Z","event":"tool.call.completed","session_id":"session-1","run_id":"run-1","status":"success","reason":"` + sensitive + `","arguments":"` + sensitive + `","result":"` + sensitive + `","path":"` + sensitive + `","error":"` + sensitive + `"}` + "\n" +
		`{"time":"2026-08-13T12:00:01Z","event":"` + sensitive + `","session_id":"session-1","run_id":"run-1","status":"failed"}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := ReadDiagnosticReport(path, DiagnosticQuery{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || strings.Contains(string(encoded), sensitive) {
		t.Fatalf("diagnostic projection leaked an unapproved field: %s", encoded)
	}
}
