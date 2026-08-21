package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestTodoProjectionReplacesIgnoresFailuresAndClearsOnTurnStart(t *testing.T) {
	projection := newTodoProjectionUnit()
	projection.ApplyProjection(transcript.ProjectionEvent{
		Entry: transcript.NewTurnStart("run-1", "turn-1"),
	})
	assertTodoProjection(t, projection, nil)

	first := tools.TodoSnapshot{Todos: []tools.TodoItem{
		{Content: "Inspect authentication", Status: tools.TodoInProgress},
		{Content: "Run tests", Status: tools.TodoPending},
	}}
	projection.ApplyProjection(todoOutcomeProjectionEvent(t, "call-1", agent.ToolOutcomeSuccess, first))
	assertTodoProjection(t, projection, &TodoSnapshot{Todos: []TodoItem{
		{Content: "Inspect authentication", Status: "in_progress"},
		{Content: "Run tests", Status: "pending"},
	}})

	failed := tools.TodoSnapshot{Todos: []tools.TodoItem{
		{Content: "Wrong replacement", Status: tools.TodoCompleted},
	}}
	projection.ApplyProjection(todoOutcomeProjectionEvent(t, "call-2", agent.ToolOutcomeFailed, failed))
	assertTodoProjection(t, projection, &TodoSnapshot{Todos: []TodoItem{
		{Content: "Inspect authentication", Status: "in_progress"},
		{Content: "Run tests", Status: "pending"},
	}})

	projection.ApplyProjection(todoOutcomeProjectionEvent(
		t,
		"call-3",
		agent.ToolOutcomeSuccess,
		tools.TodoSnapshot{Todos: []tools.TodoItem{}},
	))
	assertTodoProjection(t, projection, &TodoSnapshot{Todos: []TodoItem{}})

	projection.ApplyProjection(transcript.ProjectionEvent{
		Entry: transcript.NewTurnStart("run-1", "turn-2"),
	})
	assertTodoProjection(t, projection, nil)
}

func TestSessionRestoresTodoProjectionAndClearsItOnTheNextTurn(t *testing.T) {
	ctx := context.Background()
	store := &transcript.Memory{}
	modelCalls := 0
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Store: store,
		StreamFn: func(
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
					ID: "call-todo", Name: tools.ToolNameTodoWrite,
					Arguments: map[string]any{"todos": []any{
						map[string]any{"content": "Inspect authentication", "status": "completed"},
						map[string]any{"content": "Run tests", "status": "in_progress"},
					}},
				}}
				return finalEvents(llm.EventDone, &message), nil
			}
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(ctx, "finish this work"); err != nil {
		t.Fatal(err)
	}
	want := &TodoSnapshot{Todos: []TodoItem{
		{Content: "Inspect authentication", Status: "completed"},
		{Content: "Run tests", Status: "in_progress"},
	}}
	if got := session.Todos(); !reflect.DeepEqual(got, want) {
		t.Fatalf("live todos = %#v, want %#v", got, want)
	}
	session.Close()

	entries, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundTodoOutcome := false
	for _, entry := range entries {
		if entry.Type == transcript.ToolOutcomeEntry && entry.ToolOutcome != nil &&
			entry.ToolOutcome.ToolCallID == "call-todo" {
			foundTodoOutcome = entry.ToolOutcome.DataKind == kindTodoList
		}
	}
	if !foundTodoOutcome {
		t.Fatal("transcript has no dedicated todo_list outcome")
	}

	restored, err := New(ctx, Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Store:    store,
		StreamFn: fixedResponse("next answer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got := restored.Todos(); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored todos = %#v, want %#v", got, want)
	}
	if err := restored.Prompt(ctx, "start something else"); err != nil {
		t.Fatal(err)
	}
	if got := restored.Todos(); got != nil {
		t.Fatalf("todos after next turn = %#v, want nil", got)
	}
}

func todoOutcomeProjectionEvent(
	t *testing.T,
	callID string,
	status agent.ToolOutcomeStatus,
	data tools.TodoSnapshot,
) transcript.ProjectionEvent {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return transcript.ProjectionEvent{Entry: transcript.NewToolOutcome(transcript.ToolOutcome{
		ToolCallID: callID,
		Status:     status,
		DataKind:   kindTodoList,
		Data:       raw,
	})}
}

func assertTodoProjection(
	t *testing.T,
	projection *todoProjectionUnit,
	want *TodoSnapshot,
) {
	t.Helper()
	raw, err := projection.SnapshotProjection()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := raw.(*TodoSnapshot)
	if !ok {
		t.Fatalf("projection snapshot = %T, want *TodoSnapshot", raw)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection snapshot = %#v, want %#v", got, want)
	}
}
