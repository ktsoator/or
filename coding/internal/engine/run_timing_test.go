package engine

import (
	"context"
	"testing"

	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestSessionPersistsAndReplaysRunTiming(t *testing.T) {
	ctx := context.Background()
	memory := &transcript.Memory{}
	session, err := New(ctx, Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Tools:    []tools.Tool{},
		Store:    memory,
		StreamFn: fixedResponse("answer"),
	})
	if err != nil {
		t.Fatal(err)
	}

	var events []Event
	session.Subscribe(func(event Event) { events = append(events, event) })
	if err := session.Prompt(ctx, "question"); err != nil {
		t.Fatal(err)
	}

	if len(events) < 2 || events[0].Type != RunStarted || events[len(events)-1].Type != RunCompleted {
		t.Fatalf("run boundary events = %#v", events)
	}
	completed := events[len(events)-1]
	if completed.StartedAt.IsZero() || completed.CompletedAt.Before(completed.StartedAt) {
		t.Fatalf("invalid completed timing: %#v", completed)
	}

	entries := session.Entries()
	if len(entries) != 3 || entries[2].Type != transcript.RunEntry || entries[2].Run == nil {
		t.Fatalf("entries = %#v, want user, assistant, run", entries)
	}
	if entries[2].Run.FirstEntryID != entries[0].ID {
		t.Fatalf("run first entry = %q, want %q", entries[2].Run.FirstEntryID, entries[0].ID)
	}

	history := session.History()
	want := []HistoryItemType{HistoryUser, HistoryRun, HistoryAssistant}
	if len(history) != len(want) {
		t.Fatalf("history length = %d, want %d: %#v", len(history), len(want), history)
	}
	for index, itemType := range want {
		if history[index].Type != itemType {
			t.Fatalf("history[%d] = %q, want %q", index, history[index].Type, itemType)
		}
	}
	if history[1].StartedAt.IsZero() || history[1].CompletedAt.Before(history[1].StartedAt) {
		t.Fatalf("invalid replay timing: %#v", history[1])
	}

	restored, err := New(ctx, Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Tools:    []tools.Tool{},
		Store:    memory,
		StreamFn: fixedResponse("another answer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed := restored.History()
	if len(replayed) != len(history) || replayed[1].Type != HistoryRun {
		t.Fatalf("restored history = %#v", replayed)
	}
}

func TestHistoryIncludesRunBeforeContinueAddsAMessage(t *testing.T) {
	ctx := context.Background()
	entered := make(chan struct{})
	release := make(chan struct{})

	session, err := New(ctx, Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Tools:    []tools.Tool{},
		Store:    &transcript.Memory{},
		StreamFn: fixedResponse("answer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(ctx, "question"); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- session.run(ctx, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	history := session.History()
	if len(history) == 0 {
		t.Fatal("history is empty during Continue")
	}
	activeRun := history[len(history)-1]
	if activeRun.Type != HistoryRun || activeRun.StartedAt.IsZero() || !activeRun.CompletedAt.IsZero() {
		t.Fatalf("active run = %#v", activeRun)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
