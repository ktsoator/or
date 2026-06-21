package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestAgentPromptAppendsTranscript(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(textAssistant("one"))},
		{done(textAssistant("two"))},
	}}
	a := New(Options{Model: testModel, StreamFn: rec.fn()})

	if err := a.Prompt(context.Background(), "first"); err != nil {
		t.Fatalf("first prompt: %v", err)
	}
	if got := a.Snapshot().Messages; len(got) != 2 {
		t.Fatalf("after first prompt = %d messages, want 2", len(got))
	}

	if err := a.Prompt(context.Background(), "second"); err != nil {
		t.Fatalf("second prompt: %v", err)
	}
	state := a.Snapshot()
	if len(state.Messages) != 4 {
		t.Fatalf("after second prompt = %d messages, want 4", len(state.Messages))
	}
	if state.IsStreaming {
		t.Fatal("IsStreaming should be false after a prompt completes")
	}
	if assistant := assistantOf(t, state.Messages[3]); textOf(assistant) != "two" {
		t.Fatalf("last assistant text = %q, want %q", textOf(assistant), "two")
	}
}

func TestAgentSubscribeReceivesEvents(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(textAssistant("hi"))},
		{done(textAssistant("again"))},
	}}
	a := New(Options{Model: testModel, StreamFn: rec.fn()})

	var got []AgentEventType
	unsubscribe := a.Subscribe(func(event AgentEvent) {
		got = append(got, event.Type)
	})

	if err := a.Prompt(context.Background(), "hi"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if len(got) == 0 || got[0] != AgentStart || got[len(got)-1] != AgentEnd {
		t.Fatalf("events = %v, want to start with agent_start and end with agent_end", got)
	}

	unsubscribe()
	got = nil
	if err := a.Prompt(context.Background(), "again"); err != nil {
		t.Fatalf("second prompt: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("received %d events after unsubscribe, want 0", len(got))
	}
}

func TestAgentPromptToolLoop(t *testing.T) {
	executed := 0
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{"text": "hi"}))},
		{done(textAssistant("done"))},
	}}
	a := New(Options{
		Model:    testModel,
		StreamFn: rec.fn(),
		Tools:    []AgentTool{echoTool(func() { executed++ })},
	})

	if err := a.Prompt(context.Background(), "use echo"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if executed != 1 {
		t.Fatalf("tool executed %d times, want 1", executed)
	}
	if got := a.Snapshot().Messages; len(got) != 4 {
		t.Fatalf("transcript = %d messages, want 4 (prompt, assistant, result, assistant)", len(got))
	}
}

func TestAgentPromptRejectedWhileStreaming(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	streamFn := func(_ context.Context, _ llm.Model, _ llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		close(started)
		<-release
		ch := make(chan llm.Event, 1)
		ch <- done(textAssistant("ok"))
		close(ch)
		return ch, nil
	}
	a := New(Options{Model: testModel, StreamFn: streamFn})

	finished := make(chan error, 1)
	go func() { finished <- a.Prompt(context.Background(), "first") }()

	<-started
	if err := a.Prompt(context.Background(), "second"); err != errBusy {
		t.Fatalf("concurrent prompt error = %v, want errBusy", err)
	}

	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("first prompt: %v", err)
	}
}

func TestAgentSteerInjectsMessage(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{{done(textAssistant("ok"))}}}
	a := New(Options{Model: testModel, StreamFn: rec.fn()})

	// A steering message enqueued before the run is drained at the first poll,
	// before the first turn.
	a.Steer(userPrompt("steered"))

	if err := a.Prompt(context.Background(), "hi"); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	messages := a.Snapshot().Messages
	if len(messages) != 3 {
		t.Fatalf("transcript = %d messages, want 3 (prompt, steered, assistant)", len(messages))
	}
	wrapped, ok := messages[1].(llmMessage)
	if !ok {
		t.Fatalf("steered message is %T, want llmMessage", messages[1])
	}
	if _, ok := wrapped.Message.(*llm.UserMessage); !ok {
		t.Fatalf("steered message wraps %T, want *llm.UserMessage", wrapped.Message)
	}
}

func TestAgentAbortCancelsRun(t *testing.T) {
	streamFn := func(ctx context.Context, _ llm.Model, _ llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		ch := make(chan llm.Event)
		go func() {
			<-ctx.Done()
			ch <- llm.Event{Type: llm.EventError, Message: &llm.AssistantMessage{
				StopReason:   llm.StopReasonAborted,
				ErrorMessage: "aborted",
			}}
			close(ch)
		}()
		return ch, nil
	}
	a := New(Options{Model: testModel, StreamFn: streamFn})

	started := make(chan struct{})
	var once sync.Once
	a.Subscribe(func(event AgentEvent) {
		if event.Type == AgentStart {
			once.Do(func() { close(started) })
		}
	})

	result := make(chan error, 1)
	go func() { result <- a.Prompt(context.Background(), "hi") }()

	<-started
	a.Abort()

	if err := <-result; err == nil {
		t.Fatal("aborted prompt returned nil error, want non-nil")
	}
	if a.Snapshot().IsStreaming {
		t.Fatal("IsStreaming should be false after abort")
	}
}
