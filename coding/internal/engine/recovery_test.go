package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestNewRepairsInterruptedToolCallsBeforeProjection(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{entries: interruptedToolEntries()}

	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}

	entries, batches, appendCalls := store.snapshot()
	if appendCalls != 1 || len(batches) != 1 || len(batches[0]) != 7 {
		t.Fatalf(
			"recovery appends = %d calls, batch sizes %v; want one seven-entry batch",
			appendCalls,
			batchSizes(batches),
		)
	}
	if len(entries) != 14 {
		t.Fatalf("durable entries = %d, want seven original and seven repair entries", len(entries))
	}
	assertRecoveredOutcome(t, entries[8], "call-read", transcript.ToolNotStarted)
	assertRecoveredOutcome(t, entries[10], "call-write", transcript.ToolOutcomeUnknown)

	messages := session.Snapshot().Messages
	if len(messages) != 4 {
		t.Fatalf("restored model messages = %d, want user, assistant, and two repair results", len(messages))
	}
	for index, callID := range []string{"call-read", "call-write"} {
		message, ok := agent.ToLLM(messages[index+2])
		result, resultOK := message.(*llm.ToolResultMessage)
		if !ok || !resultOK || result.ToolCallID != callID || !result.IsError {
			t.Fatalf("restored repair result %d = %#v", index, message)
		}
	}

	results := make(map[string]HistoryItem)
	for _, item := range session.History() {
		if item.Type == HistoryToolResult {
			results[item.ToolCallID] = item
		}
	}
	if results["call-read"].ToolOutcome.ErrorCode != transcript.ToolNotStarted ||
		results["call-write"].ToolOutcome.ErrorCode != transcript.ToolOutcomeUnknown {
		t.Fatalf("restored history outcomes = %#v", results)
	}
}

func TestNewInterruptedToolRepairIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := &checkpointStore{entries: interruptedToolEntries()}
	options := Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	}

	if _, err := New(ctx, options); err != nil {
		t.Fatal(err)
	}
	if _, err := New(ctx, options); err != nil {
		t.Fatal(err)
	}
	_, batches, appendCalls := store.snapshot()
	if appendCalls != 1 || len(batches) != 1 {
		t.Fatalf("recovery appends after second load = %d calls, %d batches; want one", appendCalls, len(batches))
	}
}

func TestNewFailsWhenInterruptedToolRepairCannotBePersisted(t *testing.T) {
	storeErr := errors.New("recovery store unavailable")
	store := &checkpointStore{
		entries: interruptedToolEntries(),
		failErr: storeErr,
	}

	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	})
	if session != nil || !errors.Is(err, storeErr) {
		t.Fatalf("New() = %#v, %v; want nil session and recovery store error", session, err)
	}
	entries, batches, appendCalls := store.snapshot()
	if appendCalls != 1 || len(batches) != 0 || len(entries) != 7 {
		t.Fatalf(
			"failed recovery changed store: %d calls, %d batches, %d entries",
			appendCalls,
			len(batches),
			len(entries),
		)
	}
}

func TestNewRepairsToolsBeforeClosingInterruptedLifecycle(t *testing.T) {
	entries := interruptedToolEntries()
	store := &checkpointStore{entries: entries}

	if _, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	}); err != nil {
		t.Fatal(err)
	}

	got, batches, appendCalls := store.snapshot()
	if appendCalls != 1 || len(batches) != 1 || len(got) != 14 {
		t.Fatalf("repaired store = %d calls, %d batches, %d entries", appendCalls, len(batches), len(got))
	}
	wantTail := []transcript.EntryType{
		transcript.MessageEntry,
		transcript.ToolOutcomeEntry,
		transcript.MessageEntry,
		transcript.ToolOutcomeEntry,
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	}
	for index, want := range wantTail {
		entry := got[len(entries)+index]
		if entry.Type != want {
			t.Fatalf("repair tail[%d] = %q, want %q", index, entry.Type, want)
		}
	}
	for _, index := range []int{11, 12, 13} {
		if got[index].Lifecycle.Status != transcript.LifecycleInterrupted ||
			got[index].Lifecycle.Reason != transcript.LifecycleInterruptedReason {
			t.Fatalf("repaired lifecycle entry = %#v", got[index])
		}
	}
	if _, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	}); err != nil {
		t.Fatal(err)
	}
	_, _, appendCalls = store.snapshot()
	if appendCalls != 1 {
		t.Fatalf("second load append calls = %d, want 1", appendCalls)
	}
}

func interruptedToolEntries() []transcript.Entry {
	assistant := &llm.AssistantMessage{
		StopReason: llm.StopReasonToolUse,
		Content: []llm.AssistantContent{
			&llm.ToolCall{ID: "call-read", Name: "read", Arguments: map[string]any{"path": "one"}},
			&llm.ToolCall{ID: "call-write", Name: "write", Arguments: map[string]any{"path": "two"}},
		},
	}
	entries := []transcript.Entry{
		transcript.NewRunStart("run-1"),
		transcript.NewTurnStart("run-1", "turn-1"),
		transcript.NewMessage(agent.UserMessage("work")),
		transcript.NewStepStart("run-1", "turn-1", "step-1"),
		transcript.NewContext(transcript.ContextAttachment{
			AttachmentID: "base-context",
			Epoch:        1,
			Kind:         "base",
			Placement:    "system",
			Revision:     "revision",
			Rendered:     "context",
		}),
		transcript.NewMessage(agent.FromLLM(assistant)),
		transcript.NewToolCall(transcript.ToolCall{
			ToolCallID: "call-write",
			ToolName:   "write",
			Arguments:  []byte(`{"path":"two"}`),
		}),
	}
	sequenced, err := transcript.SequenceEntries(entries, 0)
	if err != nil {
		panic(err)
	}
	return sequenced
}

func assertRecoveredOutcome(
	t *testing.T,
	entry transcript.Entry,
	callID string,
	errorCode string,
) {
	t.Helper()
	if entry.Type != transcript.ToolOutcomeEntry || entry.ToolOutcome == nil ||
		entry.ToolOutcome.ToolCallID != callID ||
		entry.ToolOutcome.Status != agent.ToolOutcomeFailed ||
		entry.ToolOutcome.ErrorCode != errorCode {
		t.Fatalf("recovered outcome = %#v", entry)
	}
}
