package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/agent/harness"
	"github.com/ktsoator/or/llm"
)

func userMsg(text string) agent.AgentMessage {
	return agent.FromLLM(&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: text}}})
}

func assistantMsg(text string) agent.AgentMessage {
	return agent.FromLLM(&llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content:    []llm.AssistantContent{&llm.TextContent{Text: text}},
	})
}

func messageText(t *testing.T, m agent.AgentMessage) string {
	t.Helper()
	llmMsg, ok := agent.ToLLM(m)
	if !ok {
		t.Fatalf("message is not an llm message: %#v", m)
	}
	user, ok := llmMsg.(*llm.UserMessage)
	if !ok {
		return ""
	}
	var parts []string
	for _, block := range user.Content {
		if text, ok := block.(*llm.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, " ")
}

// smallWindowModel keeps the context window tiny so token thresholds trigger on
// short test transcripts.
var smallWindowModel = llm.Model{ID: "m", Provider: "p", Protocol: llm.ProtocolOpenAICompletions, ContextWindow: 100}

func stubSummarizer(summary string) harness.SummarizeFunc {
	return func(context.Context, llm.Model, []agent.AgentMessage) (string, error) {
		return summary, nil
	}
}

func TestCompactorBelowThresholdIsNoop(t *testing.T) {
	ctx := context.Background()
	called := false
	compactor := &harness.SummarizingCompactor{
		Model:    llm.Model{ContextWindow: 10000},
		Settings: harness.CompactionSettings{ReserveTokens: 10, KeepRecentTokens: 20},
		Summarize: func(context.Context, llm.Model, []agent.AgentMessage) (string, error) {
			called = true
			return "SUMMARY", nil
		},
	}

	messages := []agent.AgentMessage{
		userMsg(strings.Repeat("x", 200)),
		assistantMsg(strings.Repeat("y", 200)),
	}
	out, err := compactor.Compact(ctx, messages)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if len(out) != len(messages) {
		t.Fatalf("returned %d messages, want %d (unchanged)", len(out), len(messages))
	}
	if called {
		t.Fatal("summarizer ran below the threshold")
	}
}

func TestCompactorSummarizesOlderPrefix(t *testing.T) {
	ctx := context.Background()
	compactor := &harness.SummarizingCompactor{
		Model:     smallWindowModel,
		Settings:  harness.CompactionSettings{ReserveTokens: 10, KeepRecentTokens: 20},
		Summarize: stubSummarizer("SUMMARY TEXT"),
	}

	// ~50/50/50/10 tokens; window 100 - reserve 10 = 90 triggers; keepRecent 20
	// cuts at the third message (a user message).
	messages := []agent.AgentMessage{
		userMsg(strings.Repeat("a", 200)),
		assistantMsg(strings.Repeat("b", 200)),
		userMsg(strings.Repeat("c", 200)),
		assistantMsg(strings.Repeat("d", 40)),
	}
	out, err := compactor.Compact(ctx, messages)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("returned %d messages, want 2 (summary+kept)", len(out))
	}
	if first := messageText(t, out[0]); !strings.Contains(first, "SUMMARY TEXT") {
		t.Fatalf("first kept message missing summary, got %q", first)
	}
	// The most recent assistant message is retained verbatim.
	if _, ok := agent.ToLLM(out[1]); !ok {
		t.Fatalf("kept message is not an llm message")
	}
}

func TestManualCompactRewritesTranscriptAndSession(t *testing.T) {
	ctx := context.Background()

	session := &harness.InMemorySession{}
	seed := []agent.AgentMessage{
		userMsg(strings.Repeat("a", 400)),
		assistantMsg(strings.Repeat("b", 400)),
		userMsg(strings.Repeat("c", 400)),
		assistantMsg(strings.Repeat("d", 40)),
	}
	if err := session.Append(ctx, seed...); err != nil {
		t.Fatalf("seed Append() error = %v", err)
	}

	h, err := harness.New(ctx, harness.Options{
		Model:   smallWindowModel,
		Session: session,
		Compactor: &harness.SummarizingCompactor{
			Model:     smallWindowModel,
			Settings:  harness.CompactionSettings{ReserveTokens: 10, KeepRecentTokens: 20},
			Summarize: stubSummarizer("SUMMARY"),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	before := len(h.Snapshot().Messages)
	compacted, err := h.Compact(ctx)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if !compacted {
		t.Fatal("Compact() = false, want true")
	}

	// The in-memory transcript shrank permanently...
	after := len(h.Snapshot().Messages)
	if after >= before {
		t.Fatalf("transcript = %d after compact, want fewer than %d", after, before)
	}
	// ...and the session was rewritten to match, not appended to.
	stored, err := session.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(stored) != after {
		t.Fatalf("session has %d messages, want %d (rewritten to match transcript)", len(stored), after)
	}
}

func TestManualCompactBelowThresholdIsNoop(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{
		Model: llm.Model{ContextWindow: 10000},
		Compactor: &harness.SummarizingCompactor{
			Model:     llm.Model{ContextWindow: 10000},
			Summarize: stubSummarizer("SUMMARY"),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	// Seed a short transcript via a run-free path: a manual compact with nothing
	// over threshold should report no change.
	compacted, err := h.Compact(ctx)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if compacted {
		t.Fatal("Compact() = true on an empty transcript, want false")
	}
}

func TestManualCompactWithoutCompactorErrors(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{Model: smallWindowModel})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := h.Compact(ctx); err == nil {
		t.Fatal("Compact() = nil error without a compactor, want error")
	}
}

func TestCompactionShrinksProjectionThroughHarness(t *testing.T) {
	ctx := context.Background()

	session := &harness.InMemorySession{}
	seed := []agent.AgentMessage{
		userMsg(strings.Repeat("a", 400)),
		assistantMsg(strings.Repeat("b", 400)),
		userMsg(strings.Repeat("c", 400)),
		assistantMsg(strings.Repeat("d", 40)),
	}
	if err := session.Append(ctx, seed...); err != nil {
		t.Fatalf("seed Append() error = %v", err)
	}

	rec := &recordingStream{turns: [][]llm.Event{textTurn("ok")}}
	compactor := &harness.SummarizingCompactor{
		Model:     smallWindowModel,
		Settings:  harness.CompactionSettings{ReserveTokens: 10, KeepRecentTokens: 20},
		Summarize: stubSummarizer("SUMMARY"),
	}

	h, err := harness.New(ctx, harness.Options{
		Model:     smallWindowModel,
		StreamFn:  rec.fn(),
		Session:   session,
		Compactor: compactor,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.Prompt(ctx, "next"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	// Pre-turn transcript was the 4 seed messages plus the new prompt = 5; the
	// model must have seen fewer than that after compaction.
	if len(rec.messageCounts) != 1 {
		t.Fatalf("model ran %d times, want 1", len(rec.messageCounts))
	}
	if rec.messageCounts[0] >= 5 {
		t.Fatalf("model saw %d messages, want fewer than 5 (compacted)", rec.messageCounts[0])
	}
	// The stored transcript keeps full history; compaction is projection-only.
	if got := len(h.Snapshot().Messages); got != 6 {
		t.Fatalf("transcript has %d messages, want 6 (full history retained)", got)
	}
}
