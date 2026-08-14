package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/requestsnapshot"
	"github.com/ktsoator/or/llm"
)

func TestDiagnosticRunsEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observability.jsonl")
	recorder, err := observability.NewJSONL(path, observability.FileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(observability.Event{
		Name: observability.RunCompleted, SessionID: "session-1", RunID: "run-1",
		Status: "completed", Timestamp: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
		Duration: 250 * time.Millisecond,
	})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/runs?sessionId=session-1", nil)
	response := httptest.NewRecorder()
	NewServer(Options{ObservabilityLogPath: path}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	var report observability.DiagnosticReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 1 || report.Runs[0].ID != "run-1" || report.Runs[0].DurationMS != 250 {
		t.Fatalf("report = %#v", report)
	}
}

func TestDiagnosticRunsEndpointRejectsInvalidLimit(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/runs?limit=zero", nil)
	response := httptest.NewRecorder()
	NewServer(Options{}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestDiagnosticRequestReturnsCorrelatedSnapshot(t *testing.T) {
	store, err := requestsnapshot.NewFileStore(t.TempDir(), requestsnapshot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := requestsnapshot.NewSnapshot(
		"session-1", "run-1", "turn-1", "request-1", "test", "model",
		llm.Context{SystemPrompt: "system", Messages: []llm.Message{llm.UserText("question")}}, nil,
	)
	if err := store.Save(snapshot); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{RequestSnapshots: store})
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/diagnostics/requests/request-1?sessionId=session-1&runId=run-1",
		nil,
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var got requestsnapshot.Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProviderRequestID != "request-1" || got.RunID != "run-1" ||
		got.Input.SystemPrompt != "system" || got.Input.Messages[0].Content[0].Text != "question" {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestDiagnosticRequestRejectsMismatchedRun(t *testing.T) {
	store, err := requestsnapshot.NewFileStore(t.TempDir(), requestsnapshot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(requestsnapshot.NewSnapshot(
		"session-1", "run-1", "turn-1", "request-1", "test", "model", llm.Context{}, nil,
	)); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Options{RequestSnapshots: store})
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/diagnostics/requests/request-1?sessionId=session-1&runId=run-other",
		nil,
	)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestDiagnosticRequestReturnsUnavailableForHistoricalRequest(t *testing.T) {
	server := NewServer(Options{})
	request := httptest.NewRequest(http.MethodGet, "/api/diagnostics/requests/request-old", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
