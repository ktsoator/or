package agent

import (
	"context"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestAgentQueueHelpers(t *testing.T) {
	rec := &recorder{}
	a := New(Options{Model: testModel, StreamFn: rec.fn()})

	if a.HasQueuedMessages() {
		t.Fatal("new agent should have no queued messages")
	}
	a.Steer(userPrompt("s"))
	a.FollowUp(userPrompt("f"))
	if !a.HasQueuedMessages() {
		t.Fatal("expected queued messages after Steer and FollowUp")
	}

	a.ClearSteeringQueue()
	if !a.HasQueuedMessages() {
		t.Fatal("follow-up message should remain after clearing only steering")
	}
	a.ClearFollowUpQueue()
	if a.HasQueuedMessages() {
		t.Fatal("ClearFollowUpQueue should empty the remaining queue")
	}
}

func TestAgentReset(t *testing.T) {
	rec := &recorder{steps: [][]llm.Event{{done(textAssistant("hi"))}}}
	a := New(Options{Model: testModel, StreamFn: rec.fn()})

	if err := a.Prompt(context.Background(), "hi"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	a.Steer(userPrompt("s"))
	if len(a.Snapshot().Messages) == 0 {
		t.Fatal("expected a transcript before reset")
	}

	a.Reset()

	state := a.Snapshot()
	if len(state.Messages) != 0 {
		t.Fatalf("after reset = %d messages, want 0", len(state.Messages))
	}
	if a.HasQueuedMessages() {
		t.Fatal("Reset should clear queues")
	}
}

func TestAgentStreamOptionsReachStream(t *testing.T) {
	var seen llm.StreamOptions
	streamFn := func(_ context.Context, _ llm.Model, _ llm.Context, options llm.StreamOptions) (<-chan llm.Event, error) {
		seen = options
		ch := make(chan llm.Event, 1)
		ch <- done(textAssistant("ok"))
		close(ch)
		return ch, nil
	}
	temperature := 0.5
	a := New(Options{
		Model:         testModel,
		ThinkingLevel: "high",
		StreamFn:      streamFn,
		StreamOptions: llm.StreamOptions{
			Temperature: &temperature,
			MaxTokens:   1024,
			OnRequest:   func(string, string, []byte) {},
			Reasoning:   "low", // ThinkingLevel should win over this
		},
	})

	if err := a.Prompt(context.Background(), "hi"); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	if seen.Temperature == nil || *seen.Temperature != 0.5 {
		t.Fatalf("Temperature = %v, want 0.5", seen.Temperature)
	}
	if seen.MaxTokens != 1024 {
		t.Fatalf("MaxTokens = %d, want 1024", seen.MaxTokens)
	}
	if seen.OnRequest == nil {
		t.Fatal("OnRequest was not passed through")
	}
	if seen.Reasoning != "high" {
		t.Fatalf("Reasoning = %q, want %q (ThinkingLevel overrides StreamOptions)", seen.Reasoning, "high")
	}
}
