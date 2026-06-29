package harness_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/agent/harness"
	"github.com/ktsoator/or/llm"
)

var testModel = llm.Model{ID: "m", Provider: "p", Protocol: llm.ProtocolOpenAICompletions}

// scriptedStream returns one text assistant turn per call, each ending the run.
func scriptedStream(texts ...string) agent.StreamFn {
	calls := 0
	return func(context.Context, llm.Model, llm.Context, llm.StreamOptions) (<-chan llm.Event, error) {
		message := &llm.AssistantMessage{
			StopReason: llm.StopReasonStop,
			Content:    []llm.AssistantContent{&llm.TextContent{Text: texts[calls]}},
		}
		calls++
		ch := make(chan llm.Event, 1)
		ch <- llm.Event{Type: llm.EventDone, Message: message}
		close(ch)
		return ch, nil
	}
}

// recordingStream returns scripted turns and records the system prompt the model
// saw on each call.
type recordingStream struct {
	turns         [][]llm.Event
	calls         int
	prompts       []string
	messageCounts []int
}

func (r *recordingStream) fn() agent.StreamFn {
	return func(_ context.Context, _ llm.Model, input llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		r.prompts = append(r.prompts, input.SystemPrompt)
		r.messageCounts = append(r.messageCounts, len(input.Messages))
		turn := r.turns[r.calls]
		r.calls++
		ch := make(chan llm.Event, len(turn))
		for _, event := range turn {
			ch <- event
		}
		close(ch)
		return ch, nil
	}
}

func toolCallTurn(id, name string) []llm.Event {
	message := &llm.AssistantMessage{
		StopReason: llm.StopReasonToolUse,
		Content:    []llm.AssistantContent{&llm.ToolCall{ID: id, Name: name, Arguments: map[string]any{}}},
	}
	return []llm.Event{{Type: llm.EventDone, Message: message}}
}

func textTurn(text string) []llm.Event {
	message := &llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content:    []llm.AssistantContent{&llm.TextContent{Text: text}},
	}
	return []llm.Event{{Type: llm.EventDone, Message: message}}
}

func noopTool() agent.AgentTool {
	return agent.AgentTool{
		Definition: llm.MustTool[struct{}]("noop", "does nothing"),
		Execute: func(context.Context, string, json.RawMessage, func(agent.ToolResult)) (agent.ToolResult, error) {
			return agent.ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "done"}}}, nil
		},
	}
}

func TestPromptPersistsTranscript(t *testing.T) {
	ctx := context.Background()
	session := &harness.InMemorySession{}

	h, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: scriptedStream("hi there"),
		Session:  session,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := h.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	// The run appends the user prompt and the assistant reply, in order.
	stored, err := session.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("persisted %d messages, want 2", len(stored))
	}
}

func TestResumeSeedsFromSession(t *testing.T) {
	ctx := context.Background()
	session := &harness.InMemorySession{}

	first, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: scriptedStream("first reply"),
		Session:  session,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := first.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	// A fresh Harness over the same session resumes the prior transcript.
	second, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: scriptedStream("second reply"),
		Session:  session,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := len(second.Snapshot().Messages); got != 2 {
		t.Fatalf("resumed transcript has %d messages, want 2", got)
	}

	if err := second.Prompt(ctx, "again"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	// Resumed run appends without re-persisting the seeded messages.
	stored, err := session.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != 4 {
		t.Fatalf("persisted %d messages, want 4", len(stored))
	}
}

func TestDynamicSystemPromptReachesModel(t *testing.T) {
	ctx := context.Background()
	rec := &recordingStream{turns: [][]llm.Event{textTurn("ok")}}

	h, err := harness.New(ctx, harness.Options{
		Model:             testModel,
		StreamFn:          rec.fn(),
		BuildSystemPrompt: func(harness.TurnInfo) string { return "dynamic prompt" },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if len(rec.prompts) != 1 || rec.prompts[0] != "dynamic prompt" {
		t.Fatalf("system prompts seen = %#v, want [\"dynamic prompt\"]", rec.prompts)
	}
}

func TestSystemPromptRebuiltEachTurn(t *testing.T) {
	ctx := context.Background()
	// Turn one calls a tool; turn two ends the run. The builder keys off the
	// transcript length, so the two turns must see different prompts.
	rec := &recordingStream{turns: [][]llm.Event{
		toolCallTurn("call-1", "noop"),
		textTurn("done"),
	}}

	h, err := harness.New(ctx, harness.Options{
		Model:             testModel,
		StreamFn:          rec.fn(),
		Tools:             []agent.AgentTool{noopTool()},
		BuildSystemPrompt: func(info harness.TurnInfo) string { return fmt.Sprintf("msgs=%d", len(info.Messages)) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if len(rec.prompts) != 2 {
		t.Fatalf("saw %d turns, want 2: %#v", len(rec.prompts), rec.prompts)
	}
	if rec.prompts[0] == rec.prompts[1] {
		t.Fatalf("system prompt not rebuilt between turns: both %q", rec.prompts[0])
	}
}

func TestNoSessionStillRuns(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: scriptedStream("ok"),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if got := len(h.Snapshot().Messages); got != 2 {
		t.Fatalf("transcript has %d messages, want 2", got)
	}
}
