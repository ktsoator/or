package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	compactpkg "github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type recordingCompactor struct {
	requests []compactpkg.Request
	err      error
}

func (c *recordingCompactor) Compact(_ context.Context, request compactpkg.Request) (compactpkg.Response, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return compactpkg.Response{}, c.err
	}
	return compactpkg.Response{Summary: "summary " + string(rune('1'+len(c.requests)-1))}, nil
}

func TestSessionCompactPersistsProjectionAndCanRepeat(t *testing.T) {
	ctx := context.Background()
	memory := &transcript.Memory{}
	seed := seededTurns(6)
	if err := memory.Append(ctx, seed...); err != nil {
		t.Fatal(err)
	}
	compactor := &recordingCompactor{}
	session, err := New(ctx, Options{
		Model:     llm.Model{Provider: "test", ID: "model", ContextWindow: 400, MaxTokens: 100},
		Tools:     []tools.Tool{},
		Store:     memory,
		Compactor: compactor,
		StreamFn:  fixedResponse("new answer " + strings.Repeat("x", 120)),
	})
	if err != nil {
		t.Fatal(err)
	}
	originalCount := len(session.Messages())

	first, err := session.Compact(ctx, "focus on tests")
	if err != nil {
		t.Fatal(err)
	}
	if first.Summary != "summary 1" || len(compactor.requests) != 1 {
		t.Fatalf("first compaction = %#v, requests=%d", first, len(compactor.requests))
	}
	if len(session.Messages()) != originalCount {
		t.Fatal("compaction removed original history")
	}
	if len(session.Entries()) != len(seed)+1 {
		t.Fatal("compaction boundary was not appended")
	}
	if got := len(session.Snapshot().Messages); got >= originalCount {
		t.Fatalf("projected context was not shortened: %d >= %d", got, originalCount)
	}

	restored, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model", ContextWindow: 400, MaxTokens: 100},
		Tools: []tools.Tool{}, Store: memory, Compactor: compactor,
		StreamFn: fixedResponse("new answer " + strings.Repeat("x", 120)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Snapshot().Messages) != len(session.Snapshot().Messages) {
		t.Fatal("restored session did not rebuild compacted projection")
	}
	if err := restored.Prompt(ctx, "new request "+strings.Repeat("x", 120)); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Compact(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) != 2 || compactor.requests[1].PreviousSummary != "summary 1" {
		t.Fatalf("second compaction did not merge prior summary: %#v", compactor.requests)
	}
	compactions := 0
	for _, entry := range restored.Entries() {
		if entry.Type == transcript.CompactionEntry {
			compactions++
		}
	}
	if compactions != 2 {
		t.Fatalf("compaction entries = %d, want 2", compactions)
	}
}

func TestSessionCompactFailureDoesNotChangeState(t *testing.T) {
	ctx := context.Background()
	memory := &transcript.Memory{}
	seed := seededTurns(6)
	if err := memory.Append(ctx, seed...); err != nil {
		t.Fatal(err)
	}
	compactor := &recordingCompactor{err: errors.New("summary unavailable")}
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model", ContextWindow: 400},
		Tools: []tools.Tool{}, Store: memory, Compactor: compactor,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries := len(session.Entries())
	beforeContext := len(session.Snapshot().Messages)
	if _, err := session.Compact(ctx, ""); !errors.Is(err, compactor.err) {
		t.Fatalf("compact error = %v", err)
	}
	if len(session.Entries()) != beforeEntries || len(session.Snapshot().Messages) != beforeContext {
		t.Fatal("failed compaction changed session state")
	}
	stored, err := memory.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != beforeEntries {
		t.Fatal("failed compaction changed durable state")
	}
}

type failingAppendStore struct {
	entries []transcript.Entry
	err     error
}

func (s *failingAppendStore) Load(context.Context) ([]transcript.Entry, error) {
	return append([]transcript.Entry(nil), s.entries...), nil
}

func (s *failingAppendStore) Append(context.Context, ...transcript.Entry) error {
	return s.err
}

func TestSessionCompactStoreFailureDoesNotInstallProjection(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("disk unavailable")
	backing := &failingAppendStore{entries: seededTurns(6), err: storeErr}
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model", ContextWindow: 400},
		Tools: []tools.Tool{}, Store: backing, Compactor: &recordingCompactor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries := len(session.Entries())
	beforeContext := len(session.Snapshot().Messages)
	if _, err := session.Compact(ctx, ""); !errors.Is(err, storeErr) {
		t.Fatalf("compact error = %v", err)
	}
	if len(session.Entries()) != beforeEntries || len(session.Snapshot().Messages) != beforeContext {
		t.Fatal("store failure installed a partial compaction")
	}
}

func seededTurns(count int) []transcript.Entry {
	entries := make([]transcript.Entry, 0, count*2)
	for index := 0; index < count; index++ {
		messages := []agent.AgentMessage{
			agent.UserMessage("request " + strings.Repeat("u", 120)),
			agent.FromLLM(&llm.AssistantMessage{
				Content:    []llm.AssistantContent{&llm.TextContent{Text: "answer " + strings.Repeat("a", 120)}},
				StopReason: llm.StopReasonStop,
			}),
		}
		for _, message := range messages {
			entries = append(entries, transcript.NewMessage(message))
		}
	}
	return entries
}

func fixedResponse(text string) agent.StreamFn {
	return func(_ context.Context, model llm.Model, _ llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		message := llm.NewAssistantMessage(model)
		message.Content = []llm.AssistantContent{&llm.TextContent{Text: text}}
		message.StopReason = llm.StopReasonStop
		events := make(chan llm.Event, 1)
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	}
}
