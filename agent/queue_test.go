package agent

import (
	"context"
	"testing"

	"github.com/ktsoator/or/llm"
)

// userText unwraps a queued user message back to its text for assertions.
func userText(t *testing.T, message AgentMessage) string {
	t.Helper()
	wrapped, ok := message.(llmMessage)
	if !ok {
		t.Fatalf("not an llm message: %T", message)
	}
	user, ok := wrapped.Message.(*llm.UserMessage)
	if !ok {
		t.Fatalf("not a user message: %T", wrapped.Message)
	}
	for _, block := range user.Content {
		if text, ok := block.(*llm.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func TestMessageQueueDrainAll(t *testing.T) {
	q := &messageQueue{mode: QueueAll}
	q.enqueue(userPrompt("1"))
	q.enqueue(userPrompt("2"))

	drained := q.drain()
	if len(drained) != 2 {
		t.Fatalf("drained = %d messages, want 2", len(drained))
	}
	if len(q.drain()) != 0 {
		t.Fatal("queue should be empty after draining all")
	}
}

func TestMessageQueueDrainOneAtATime(t *testing.T) {
	q := &messageQueue{mode: QueueOneAtATime}
	q.enqueue(userPrompt("1"))
	q.enqueue(userPrompt("2"))

	first := q.drain()
	if len(first) != 1 || userText(t, first[0]) != "1" {
		t.Fatalf("first drain = %v, want one message %q", first, "1")
	}
	second := q.drain()
	if len(second) != 1 || userText(t, second[0]) != "2" {
		t.Fatalf("second drain = %v, want one message %q", second, "2")
	}
	if len(q.drain()) != 0 {
		t.Fatal("queue should be empty after draining both")
	}
}

func TestAgentCancelQueuedRemovesOnlyItsHandle(t *testing.T) {
	a := New(Options{Model: testModel})
	first := a.Steer(userPrompt("same"))
	second := a.Steer(userPrompt("same"))

	if !a.CancelQueued(first) {
		t.Fatal("CancelQueued(first) = false")
	}
	if a.CancelQueued(first) {
		t.Fatal("cancelled the same handle twice")
	}
	drained := a.steering.drain()
	if len(drained) != 1 || userText(t, drained[0]) != "same" {
		t.Fatalf("drained = %#v", drained)
	}
	if a.CancelQueued(second) {
		t.Fatal("cancelled a handle after it was drained")
	}
}

func TestAgentSteeringOneAtATimeSpansTurns(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(textAssistant("a1"))},
		{done(textAssistant("a2"))},
	}}
	a := New(Options{Model: testModel, StreamFn: rec.fn(), SteeringMode: QueueOneAtATime})
	a.Steer(userPrompt("s1"))
	a.Steer(userPrompt("s2"))

	if err := a.Prompt(context.Background(), "go"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if rec.calls != 2 {
		t.Fatalf("stream calls = %d, want 2 (one steering message injected per turn)", rec.calls)
	}
}

func TestAgentSteeringAllInjectsTogether(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(textAssistant("a1"))},
	}}
	a := New(Options{Model: testModel, StreamFn: rec.fn(), SteeringMode: QueueAll})
	a.Steer(userPrompt("s1"))
	a.Steer(userPrompt("s2"))

	if err := a.Prompt(context.Background(), "go"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if rec.calls != 1 {
		t.Fatalf("stream calls = %d, want 1 (all steering injected at once)", rec.calls)
	}
}

func TestAgentDefaultSteeringModeIsOneAtATime(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(textAssistant("a1"))},
		{done(textAssistant("a2"))},
	}}
	a := New(Options{Model: testModel, StreamFn: rec.fn()}) // no mode set → default
	a.Steer(userPrompt("s1"))
	a.Steer(userPrompt("s2"))

	if err := a.Prompt(context.Background(), "go"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if rec.calls != 2 {
		t.Fatalf("stream calls = %d, want 2 (default one-at-a-time spreads steering over turns)", rec.calls)
	}
}
