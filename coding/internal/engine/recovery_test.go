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
	if appendCalls != 1 || len(batches) != 1 || len(batches[0]) != 4 {
		t.Fatalf(
			"recovery appends = %d calls, batch sizes %v; want one four-entry batch",
			appendCalls,
			batchSizes(batches),
		)
	}
	if len(entries) != 8 {
		t.Fatalf("durable entries = %d, want four original and four repair entries", len(entries))
	}
	assertRecoveredOutcome(t, entries[5], "call-read", transcript.ToolNotStarted)
	assertRecoveredOutcome(t, entries[7], "call-write", transcript.ToolOutcomeUnknown)

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
	if appendCalls != 1 || len(batches) != 0 || len(entries) != 4 {
		t.Fatalf(
			"failed recovery changed store: %d calls, %d batches, %d entries",
			appendCalls,
			len(batches),
			len(entries),
		)
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
	return []transcript.Entry{
		transcript.NewContext(transcript.ContextAttachment{
			AttachmentID: "base-context",
			Epoch:        1,
			Kind:         "base",
			Placement:    "system",
			Revision:     "revision",
			Rendered:     "context",
		}),
		transcript.NewMessage(agent.UserMessage("work")),
		transcript.NewMessage(agent.FromLLM(assistant)),
		transcript.NewToolCall(transcript.ToolCall{
			ToolCallID: "call-write",
			ToolName:   "write",
			Arguments:  []byte(`{"path":"two"}`),
		}),
	}
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
