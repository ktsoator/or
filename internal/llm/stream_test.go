package llm

import (
	"context"
	"errors"
	"testing"
)

func TestStreamWriterEmitsExactlyOneTerminalEvent(t *testing.T) {
	events := make(chan Event, 10)
	message := AssistantMessage{}
	writer := NewStreamWriter(context.Background(), events, &message)

	writer.Emit(Event{Type: EventTextDelta, Delta: "hello"})
	writer.Done()
	writer.Fail(errors.New("late failure"))
	writer.Emit(Event{Type: EventTextDelta, Delta: "late delta"})
	writer.Done()

	got := drainEvents(events)
	want := []EventType{EventStart, EventTextDelta, EventDone}
	if len(got) != len(want) {
		t.Fatalf("events = %#v, want types %v", got, want)
	}
	for i, eventType := range want {
		if got[i].Type != eventType {
			t.Fatalf("event[%d].Type = %q, want %q", i, got[i].Type, eventType)
		}
	}
}

func TestStreamWriterFailWithoutError(t *testing.T) {
	events := make(chan Event, 2)
	message := AssistantMessage{}
	writer := NewStreamWriter(context.Background(), events, &message)

	writer.Fail(nil)

	got := drainEvents(events)
	if len(got) != 2 || got[0].Type != EventStart || got[1].Type != EventError {
		t.Fatalf("events = %#v, want start then error", got)
	}
	terminal := got[1]
	if terminal.Err == nil || terminal.Message == nil || terminal.Message.StopReason != StopReasonError {
		t.Fatalf("terminal event = %#v, want ordinary stream error", terminal)
	}
}

func TestStreamWriterDoneAfterCancellationEmitsAbortedError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events := make(chan Event, 2)
	message := AssistantMessage{}
	writer := NewStreamWriter(ctx, events, &message)

	writer.Done()

	got := drainEvents(events)
	if len(got) != 2 || got[0].Type != EventStart || got[1].Type != EventError {
		t.Fatalf("events = %#v, want start then error", got)
	}
	terminal := got[1]
	if !errors.Is(terminal.Err, context.Canceled) || terminal.Message == nil ||
		terminal.Message.StopReason != StopReasonAborted {
		t.Fatalf("terminal event = %#v, want cancelled stream", terminal)
	}
}

func TestStreamWriterStartIsIdempotent(t *testing.T) {
	events := make(chan Event, 4)
	message := AssistantMessage{}
	writer := NewStreamWriter(context.Background(), events, &message)

	writer.Start()
	writer.Start() // second call must not emit
	writer.Done()

	got := drainEvents(events)
	if len(got) != 2 || got[0].Type != EventStart || got[1].Type != EventDone {
		t.Fatalf("events = %#v, want start then done", got)
	}
}

func TestStreamWriterStartAfterFinishIsNoop(t *testing.T) {
	events := make(chan Event, 4)
	message := AssistantMessage{}
	writer := NewStreamWriter(context.Background(), events, &message)

	writer.Done()
	writer.Start() // must not emit after terminal

	got := drainEvents(events)
	if len(got) != 2 {
		t.Fatalf("events = %#v, want start+done only", got)
	}
}

func TestCloneToolCallIsDeep(t *testing.T) {
	original := &ToolCall{
		ID:   "x",
		Name: "search",
		Arguments: map[string]any{
			"query":   "hello",
			"options": map[string]any{"limit": 10.0},
			"tags":    []any{"a", "b"},
		},
	}
	clone := CloneToolCall(original)
	if clone == original {
		t.Fatalf("CloneToolCall returned the same pointer")
	}

	// Mutate the clone deeply; the original must stay intact.
	clone.Arguments["query"] = "mutated"
	clone.Arguments["options"].(map[string]any)["limit"] = 99.0
	clone.Arguments["tags"].([]any)[0] = "z"

	if original.Arguments["query"] != "hello" {
		t.Fatalf("top-level argument leaked: %v", original.Arguments["query"])
	}
	if original.Arguments["options"].(map[string]any)["limit"] != 10.0 {
		t.Fatalf("nested object mutation leaked")
	}
	if original.Arguments["tags"].([]any)[0] != "a" {
		t.Fatalf("nested array mutation leaked")
	}
}

func TestCloneToolCallNilReturnsNil(t *testing.T) {
	if got := CloneToolCall(nil); got != nil {
		t.Fatalf("CloneToolCall(nil) = %v, want nil", got)
	}
}

func drainEvents(events chan Event) []Event {
	result := make([]Event, 0, len(events))
	for len(events) > 0 {
		result = append(result, <-events)
	}
	return result
}
