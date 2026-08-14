package tracebundle

import (
	"errors"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/requestsnapshot"
)

func TestBuildAssemblesTaskRequestsAndToolResults(t *testing.T) {
	store, err := requestsnapshot.NewFileStore(t.TempDir(), requestsnapshot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	if err := store.Save(requestsnapshot.Snapshot{
		Version: requestsnapshot.CurrentVersion, CapturedAt: base,
		SessionID: "session-1", RunID: "run-1", TurnID: "turn-1",
		ProviderRequestID: "request-1", Provider: "openai", Model: "gpt-5",
		Input: requestsnapshot.Input{
			SystemPrompt: "You are a coding agent.",
			Messages: []requestsnapshot.Message{
				{Role: "user", Content: []requestsnapshot.Content{{Type: "text", Text: "Create a file"}}},
				{Role: "user", Content: []requestsnapshot.Content{{Type: "text", Text: "runtime context"}}},
			},
			Tools: []requestsnapshot.Tool{{Name: "write", Description: "Write a file"}},
		},
		Attachments: []requestsnapshot.Attachment{{ID: "context", Kind: "context_update", MessageIndex: 1}},
		Output: &requestsnapshot.Output{
			CapturedAt: base.Add(2 * time.Second), StopReason: "tool_use",
			Message: requestsnapshot.Message{
				Role: "assistant", ProviderRequestID: "request-1",
				Content: []requestsnapshot.Content{
					{Type: "thinking", Thinking: "Use the write tool"},
					{Type: "text", Text: "I will create it."},
					{Type: "toolCall", ToolCallID: "call-1", ToolName: "write", Arguments: map[string]any{"path": "note.txt"}},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(requestsnapshot.Snapshot{
		Version: requestsnapshot.CurrentVersion, CapturedAt: base.Add(2300 * time.Millisecond),
		SessionID: "session-1", RunID: "run-1", TurnID: "turn-2",
		ProviderRequestID: "request-2", Provider: "openai", Model: "gpt-5",
		Input: requestsnapshot.Input{Messages: []requestsnapshot.Message{
			{Role: "user", Content: []requestsnapshot.Content{{Type: "text", Text: "Create a file"}}},
			{
				Role: "assistant", ProviderRequestID: "request-1",
				Content: []requestsnapshot.Content{{Type: "toolCall", ToolCallID: "call-1", ToolName: "write"}},
			},
			{
				Role: "toolResult", ToolCallID: "call-1", ToolName: "write",
				Content: []requestsnapshot.Content{{Type: "text", Text: "created note.txt"}},
			},
		}},
		Output: &requestsnapshot.Output{
			CapturedAt: base.Add(3 * time.Second), StopReason: "stop",
			Message: requestsnapshot.Message{
				Role: "assistant", ProviderRequestID: "request-2",
				Content: []requestsnapshot.Content{{Type: "text", Text: "Done"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	report := observability.DiagnosticReport{
		GeneratedAt: base.Add(4 * time.Second),
		Runs: []observability.DiagnosticRun{
			diagnosticRun("run-1", base, []observability.DiagnosticEvent{
				{Name: observability.TurnStarted, Timestamp: base, TurnID: "turn-1", Status: "running"},
				{Name: observability.CheckpointCompleted, Timestamp: base.Add(10 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", Status: "completed", DurationMS: 10},
				{Name: observability.ProviderStarted, Timestamp: base.Add(10 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", Provider: "openai", Model: "gpt-5", Status: "running"},
				{Name: observability.HTTPAttemptStarted, Timestamp: base.Add(20 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", Attempt: 1, Status: "running"},
				{Name: observability.HTTPAttemptResponse, Timestamp: base.Add(100 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", Attempt: 1, HTTPStatus: 200, Status: "completed", DurationMS: 80},
				{Name: observability.ProviderCompleted, Timestamp: base.Add(2 * time.Second), TurnID: "turn-1", ProviderRequestID: "request-1", Provider: "openai", Model: "gpt-5", Status: "completed", DurationMS: 1990, TimeToFirstOutputMS: 900, InputTokens: 100, OutputTokens: 30, TotalTokens: 130},
				{Name: observability.ToolStarted, Timestamp: base.Add(2010 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", ToolCallID: "call-1", ToolName: "write", Status: "running"},
				{Name: observability.ApprovalStarted, Timestamp: base.Add(2020 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", ToolCallID: "call-1", ToolName: "write", Status: "waiting"},
				{Name: observability.ApprovalCompleted, Timestamp: base.Add(2120 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", ToolCallID: "call-1", ToolName: "write", Status: "allowed", DurationMS: 100},
				{Name: observability.ToolCompleted, Timestamp: base.Add(2210 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", ToolCallID: "call-1", ToolName: "write", Status: "success", DurationMS: 200},
				{Name: observability.TurnCompleted, Timestamp: base.Add(2220 * time.Millisecond), TurnID: "turn-1", ProviderRequestID: "request-1", Status: "completed", DurationMS: 2220},
				{Name: observability.ProviderStarted, Timestamp: base.Add(2300 * time.Millisecond), TurnID: "turn-2", ProviderRequestID: "request-2", Provider: "openai", Model: "gpt-5", Status: "running"},
				{Name: observability.ProviderCompleted, Timestamp: base.Add(3 * time.Second), TurnID: "turn-2", ProviderRequestID: "request-2", Provider: "openai", Model: "gpt-5", Status: "completed", DurationMS: 700, TimeToFirstOutputMS: 400, InputTokens: 140, OutputTokens: 15, TotalTokens: 155},
			}),
			diagnosticRun("run-old", base.Add(-time.Hour), []observability.DiagnosticEvent{
				{Name: observability.ProviderCompleted, Timestamp: base.Add(-time.Hour), TurnID: "turn-old", ProviderRequestID: "request-old", Status: "completed", DurationMS: 1000},
			}),
		},
	}

	bundle, err := Build(report, "session-1", "run-1", store)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Version != CurrentVersion || bundle.SessionID != "session-1" || bundle.SelectedTaskID != "run-1" {
		t.Fatalf("bundle identity = %#v", bundle)
	}
	if len(bundle.Tasks) != 2 || bundle.Tasks[0].ID != "run-old" || bundle.Tasks[1].ID != "run-1" {
		t.Fatalf("tasks = %#v", bundle.Tasks)
	}
	task := bundle.Tasks[1]
	if task.Prompt != "Create a file" || len(task.Requests) != 2 {
		t.Fatalf("task = %#v", task)
	}
	first := task.Requests[0]
	if first.Number != 2 || first.SnapshotState != SnapshotAvailable || first.CheckpointDurationMS != 10 {
		t.Fatalf("first request = %#v", first)
	}
	if len(first.Attempts) != 1 || first.Attempts[0].HTTPStatus != 200 {
		t.Fatalf("attempts = %#v", first.Attempts)
	}
	if len(first.Tools) != 1 || first.Tools[0].ApprovalDurationMS != 100 ||
		first.Tools[0].ExecutionDurationMS != 100 || first.Tools[0].Arguments["path"] != "note.txt" {
		t.Fatalf("tools = %#v", first.Tools)
	}
	if first.Tools[0].Result == nil || first.Tools[0].Result.Content[0].Text != "created note.txt" {
		t.Fatalf("tool result = %#v", first.Tools[0].Result)
	}
	if task.Requests[1].Number != 3 || task.Requests[1].Output == nil {
		t.Fatalf("second request = %#v", task.Requests[1])
	}
}

func TestBuildKeepsRequestWhenSnapshotIsMissing(t *testing.T) {
	base := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	report := observability.DiagnosticReport{Runs: []observability.DiagnosticRun{
		diagnosticRun("run-1", base, []observability.DiagnosticEvent{{
			Name: observability.ProviderCompleted, Timestamp: base,
			TurnID: "turn-1", ProviderRequestID: "request-1",
			Status: "completed", DurationMS: 500,
		}}),
	}}
	bundle, err := Build(report, "session-1", "run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request := bundle.Tasks[0].Requests[0]
	if request.SnapshotState != SnapshotMissing || request.Lifecycle != "missing-start" {
		t.Fatalf("request = %#v", request)
	}
}

func TestBuildRejectsUnknownTask(t *testing.T) {
	_, err := Build(observability.DiagnosticReport{}, "session-1", "missing", nil)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func diagnosticRun(
	id string,
	startedAt time.Time,
	events []observability.DiagnosticEvent,
) observability.DiagnosticRun {
	updatedAt := startedAt
	if len(events) > 0 {
		updatedAt = events[len(events)-1].Timestamp
	}
	return observability.DiagnosticRun{
		ID: id, SessionID: "session-1", Status: "completed",
		StartedAt: startedAt, UpdatedAt: updatedAt, Events: events,
	}
}
