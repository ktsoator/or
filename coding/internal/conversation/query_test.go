package conversation

import (
	"context"
	"reflect"
	"testing"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestManagerReconstructsRequestSnapshotFromTranscript(t *testing.T) {
	manager := newTestManager(t, t.TempDir())
	manager.streamFn = forkResponses("answer")
	model, thinking := testCatalogModel(t)
	created, err := manager.Create(
		"Diagnostics", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(created.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, created.ID)

	var requestID string
	for _, entry := range mustRuntime(t, manager, created.ID).session.Entries() {
		if entry.Type == transcript.RequestHeaderEntry {
			requestID = entry.RequestHeader.ProviderRequestID
			break
		}
	}
	if requestID == "" {
		t.Fatal("transcript has no provider request")
	}
	record, err := manager.LoadForSession(created.ID, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if record.SessionID != created.ID || record.ProviderRequestID != requestID ||
		len(record.Input.Messages) == 0 || record.Output == nil ||
		record.Output.Message.Content[0].Text != "answer" {
		t.Fatalf("request snapshot = %#v", record)
	}
}

func TestManagerSnapshotRestoresTodoProjection(t *testing.T) {
	manager := newTestManagerWithTransport(t, t.TempDir(), func(string) Transport {
		return &idleClosingTestTransport{testTransport: &testTransport{}}
	})
	modelCalls := 0
	manager.streamFn = func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		modelCalls++
		if modelCalls == 1 {
			message := llm.NewAssistantMessage(model)
			message.StopReason = llm.StopReasonToolUse
			message.Content = []llm.AssistantContent{&llm.ToolCall{
				ID: "todo-call", Name: tools.ToolNameTodoWrite,
				Arguments: map[string]any{"todos": []any{
					map[string]any{"content": "Inspect parser", "status": "completed"},
					map[string]any{"content": "Run tests", "status": "in_progress"},
				}},
			}}
			return todoQueryEvents(&message), nil
		}
		return todoQueryEvents(todoQueryAnswer(model, "done")), nil
	}
	model, thinking := testCatalogModel(t)
	created, err := manager.Create(
		"Todo snapshot", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(created.ID, "finish the parser", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, created.ID)

	want := &engine.TodoSnapshot{Todos: []engine.TodoItem{
		{Content: "Inspect parser", Status: "completed"},
		{Content: "Run tests", Status: "in_progress"},
	}}
	snapshot, err := manager.Snapshot(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Todos, want) {
		t.Fatalf("loaded todos = %#v, want %#v", snapshot.Todos, want)
	}
	if !manager.ReleaseIfIdle(created.ID) {
		t.Fatal("idle conversation was not released")
	}
	restored, err := manager.Snapshot(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Todos, want) {
		t.Fatalf("restored todos = %#v, want %#v", restored.Todos, want)
	}
}

func todoQueryAnswer(model llm.Model, text string) *llm.AssistantMessage {
	message := llm.NewAssistantMessage(model)
	message.StopReason = llm.StopReasonStop
	message.Content = []llm.AssistantContent{&llm.TextContent{Text: text}}
	return &message
}

func todoQueryEvents(message *llm.AssistantMessage) <-chan llm.Event {
	events := make(chan llm.Event, 1)
	events <- llm.Event{Type: llm.EventDone, Message: message}
	close(events)
	return events
}
