package coding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/store"
	"github.com/ktsoator/or/llm"
)

// scriptedStream returns a StreamFn that yields the given assistant messages in
// order, one per turn. Each turn emits a start event then a terminal event: a
// done event normally, or an error event when the turn is itself an error.
func scriptedStream(turns ...*llm.AssistantMessage) agent.StreamFn {
	i := 0
	return func(_ context.Context, _ llm.Model, _ llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		final := turns[i]
		i++
		terminal := llm.Event{Type: llm.EventDone, Message: final}
		if final.StopReason == llm.StopReasonError {
			terminal = llm.Event{Type: llm.EventError, Message: final, Err: errors.New(final.ErrorMessage)}
		}
		ch := make(chan llm.Event, 2)
		ch <- llm.Event{Type: llm.EventStart, Partial: &llm.AssistantMessage{}}
		ch <- terminal
		close(ch)
		return ch, nil
	}
}

func assistantError(text string) *llm.AssistantMessage {
	return &llm.AssistantMessage{StopReason: llm.StopReasonError, ErrorMessage: text}
}

func assistantToolCall(name string, args map[string]any) *llm.AssistantMessage {
	return &llm.AssistantMessage{
		Content:    []llm.AssistantContent{&llm.ToolCall{ID: "call-1", Name: name, Arguments: args}},
		StopReason: llm.StopReasonToolUse,
	}
}

func assistantText(text string) *llm.AssistantMessage {
	return &llm.AssistantMessage{
		Content:    []llm.AssistantContent{&llm.TextContent{Text: text}},
		StopReason: llm.StopReasonStop,
	}
}

// TestSessionRunsToolAndPersists drives a full turn offline: the model asks to
// read a file, the tool executes through the permission gate, and the second
// turn produces a final answer. It checks the transcript and that the store
// captured every message.
func TestSessionRunsToolAndPersists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi there\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	memStore := &store.Memory{}
	session, err := New(context.Background(), Options{
		Model: llm.Model{ID: "test", Provider: "test"},
		Cwd:   dir,
		Store: memStore,
		StreamFn: scriptedStream(
			assistantToolCall("read", map[string]any{"path": "hello.txt"}),
			assistantText("The file says hi."),
		),
		// Default policy allows read (read-only); no Confirm needed.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var readExecuted bool
	session.Subscribe(func(ev agent.AgentEvent) {
		if ev.Type == agent.ToolEnd && ev.ToolName == "read" {
			readExecuted = true
		}
	})

	if err := session.Prompt(context.Background(), "what does hello.txt say?"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	if !readExecuted {
		t.Fatal("read tool did not execute")
	}

	// Transcript: user, assistant(toolcall), toolresult, assistant(text).
	messages := session.Messages()
	if len(messages) != 4 {
		t.Fatalf("want 4 messages, got %d", len(messages))
	}

	// The store persisted the same transcript.
	persisted, err := memStore.Load(context.Background())
	if err != nil {
		t.Fatalf("store load: %v", err)
	}
	if len(persisted) != 4 {
		t.Fatalf("want 4 persisted messages, got %d", len(persisted))
	}

	last, ok := agent.ToLLM(messages[len(messages)-1])
	if !ok {
		t.Fatal("last message has no llm projection")
	}
	assistant, ok := last.(*llm.AssistantMessage)
	if !ok {
		t.Fatalf("last message is %T", last)
	}
	if assistant.Text() != "The file says hi." {
		t.Fatalf("final answer = %q", assistant.Text())
	}
}

// TestSessionRetriesTransientError drives a turn that fails with a transient
// (rate-limit) error, then succeeds. The session should retry transparently,
// drop the failed turn, and end with the recovered answer.
func TestSessionRetriesTransientError(t *testing.T) {
	session, err := New(context.Background(), Options{
		Model: llm.Model{ID: "test", Provider: "test", ContextWindow: 100000},
		Cwd:   t.TempDir(),
		StreamFn: scriptedStream(
			assistantError("HTTP 429 too many requests"),
			assistantText("recovered"),
		),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := session.Prompt(context.Background(), "hi"); err != nil {
		t.Fatalf("retry should have recovered, got: %v", err)
	}

	// Transcript: user, assistant(recovered). The failed turn was dropped.
	messages := session.Messages()
	if len(messages) != 2 {
		t.Fatalf("want 2 messages after retry, got %d", len(messages))
	}
	if last := lastAssistant(messages); last == nil || last.Text() != "recovered" {
		t.Fatalf("final answer not recovered: %+v", last)
	}
}

// TestSessionDoesNotRetryPermanentError verifies a non-transient error is not
// retried: the failed turn stays and the error surfaces immediately.
func TestSessionDoesNotRetryPermanentError(t *testing.T) {
	session, err := New(context.Background(), Options{
		Model: llm.Model{ID: "test", Provider: "test", ContextWindow: 100000},
		Cwd:   t.TempDir(),
		StreamFn: scriptedStream(
			assistantError("invalid api key"),
			assistantText("should not reach here"),
		),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := session.Prompt(context.Background(), "hi"); err == nil {
		t.Fatal("permanent error should not be retried away")
	}
}

// TestPolicyBlocksUnconfirmedMutation verifies the default gate denies a
// workspace-changing tool when no approver is configured, and the run still
// completes with an error tool result rather than crashing.
func TestPolicyBlocksUnconfirmedMutation(t *testing.T) {
	dir := t.TempDir()

	session, err := New(context.Background(), Options{
		Model: llm.Model{ID: "test", Provider: "test"},
		Cwd:   dir,
		StreamFn: scriptedStream(
			assistantToolCall("write", map[string]any{"path": "new.txt", "content": "data"}),
			assistantText("done"),
		),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := session.Prompt(context.Background(), "create a file"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// The write must have been blocked: no file created.
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("write should have been blocked by the default policy")
	}
}
