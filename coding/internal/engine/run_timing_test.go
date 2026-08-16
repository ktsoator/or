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
	projection, err := transcript.ProjectSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Runs) != 1 || len(projection.Messages) != 2 || len(projection.Contexts) != 1 {
		t.Fatalf("projection = %#v", projection)
	}
	run := projection.Runs[0]
	userEntry := projection.Messages[0]
	assistantEntry := projection.Messages[1]
	if len(completed.UserMessageIDs) != 1 || completed.UserMessageIDs[0] != userEntry.EntryID ||
		completed.AssistantMessageID != assistantEntry.EntryID {
		t.Fatalf("completed message ids = users %v, assistant %q", completed.UserMessageIDs, completed.AssistantMessageID)
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
	if !history[2].CompletedAt.Equal(history[1].CompletedAt) {
		t.Fatalf("response completion = %v, want run completion %v", history[2].CompletedAt, history[1].CompletedAt)
	}
	if history[0].MessageID != userEntry.EntryID {
		t.Fatalf("user message id = %q, want transcript entry %q", history[0].MessageID, userEntry.EntryID)
	}
	if !history[0].SentAt.Equal(userEntry.Timestamp) {
		t.Fatalf("user message time = %v, want transcript entry time %v", history[0].SentAt, userEntry.Timestamp)
	}
	if history[1].MessageID != "" {
		t.Fatalf("run message id = %q, want empty", history[1].MessageID)
	}
	if history[1].RunID != run.ID {
		t.Fatalf("run id = %q, want projected run %q", history[1].RunID, run.ID)
	}
	if history[2].MessageID != assistantEntry.EntryID {
		t.Fatalf("assistant message id = %q, want transcript entry %q", history[2].MessageID, assistantEntry.EntryID)
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
	if !replayed[2].CompletedAt.Equal(replayed[1].CompletedAt) {
		t.Fatalf("restored response completion = %v, want run completion %v", replayed[2].CompletedAt, replayed[1].CompletedAt)
	}
	if replayed[0].MessageID != userEntry.EntryID || replayed[2].MessageID != assistantEntry.EntryID {
		t.Fatalf("restored message ids = %q/%q, want %q/%q",
			replayed[0].MessageID, replayed[2].MessageID, userEntry.EntryID, assistantEntry.EntryID)
	}
	if !replayed[0].SentAt.Equal(userEntry.Timestamp) {
		t.Fatalf("restored user message time = %v, want %v", replayed[0].SentAt, userEntry.Timestamp)
	}
}

func TestHistoryDoesNotDuplicateRunAfterCompletedEntryIsPersisted(t *testing.T) {
	ctx := context.Background()
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

	entries := session.Entries()
	projection, err := transcript.ProjectSession(entries)
	if err != nil || len(projection.Runs) != 1 {
		t.Fatalf("projected runs = %#v, %v", projection, err)
	}
	completedRun := projection.Runs[0]
	// Recreate the interval after persistRunTerminal and before the deferred active
	// run state is cleared.
	session.setRunState(ctx, completedRun.ID, "turn-test", completedRun.StartedAt)
	defer session.clearRunState()

	history := session.History()
	runs := 0
	for _, item := range history {
		if item.Type == HistoryRun {
			runs++
		}
	}
	if runs != 1 {
		t.Fatalf("history contains %d runs, want one: %#v", runs, history)
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
