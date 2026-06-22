package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ktsoator/or/llm"
)

// twoEchoCalls is an assistant turn that calls the echo tool twice.
func twoEchoCalls() *llm.AssistantMessage {
	return &llm.AssistantMessage{
		StopReason: llm.StopReasonToolUse,
		Content: []llm.AssistantContent{
			&llm.ToolCall{ID: "a", Name: "echo", Arguments: map[string]any{"text": "1"}},
			&llm.ToolCall{ID: "b", Name: "echo", Arguments: map[string]any{"text": "2"}},
		},
	}
}

// overlapTool reports the maximum number of concurrent executions observed.
func overlapTool(execMode ExecutionMode, active, maxActive *int, mu *sync.Mutex) AgentTool {
	return AgentTool{
		Definition:    llm.MustTool[echoArgs]("echo", "echo"),
		ExecutionMode: execMode,
		Execute: func(_ context.Context, _ string, args json.RawMessage, _ func(ToolResult)) (ToolResult, error) {
			mu.Lock()
			*active++
			if *active > *maxActive {
				*maxActive = *active
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond) // widen the window for overlap to show
			mu.Lock()
			*active--
			mu.Unlock()

			var parsed echoArgs
			_ = json.Unmarshal(args, &parsed)
			return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "echoed: " + parsed.Text}}}, nil
		},
	}
}

func TestExecuteToolCallsRunConcurrently(t *testing.T) {
	// A barrier that only releases once both tools have started. A sequential
	// batch would never reach the second start, deadlock, and fail by timeout.
	var started sync.WaitGroup
	started.Add(2)
	release := make(chan struct{})
	go func() {
		started.Wait()
		close(release)
	}()

	tool := AgentTool{
		Definition: llm.MustTool[echoArgs]("echo", "echo"),
		Execute: func(_ context.Context, _ string, args json.RawMessage, _ func(ToolResult)) (ToolResult, error) {
			started.Done()
			<-release
			var parsed echoArgs
			_ = json.Unmarshal(args, &parsed)
			return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "echoed: " + parsed.Text}}}, nil
		},
	}
	rec := &recorder{turns: [][]llm.Event{
		{done(twoEchoCalls())},
		{done(textAssistant("done"))},
	}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()} // default is parallel
	base := Context{Tools: []AgentTool{tool}}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("go")}, base, cfg))

	// Reaching here means both tools ran concurrently. Results stay in source
	// order regardless of completion order.
	messages := agentEndMessages(t, events)
	if len(messages) != 5 {
		t.Fatalf("messages = %d, want 5 (prompt, assistant, result a, result b, assistant)", len(messages))
	}
	if got := resultText(toolResultOf(t, messages[2]).Content); got != "echoed: 1" {
		t.Fatalf("result[0] = %q, want %q", got, "echoed: 1")
	}
	if got := resultText(toolResultOf(t, messages[3]).Content); got != "echoed: 2" {
		t.Fatalf("result[1] = %q, want %q", got, "echoed: 2")
	}
}

func TestExecuteToolCallsSequentialModeDoesNotOverlap(t *testing.T) {
	var mu sync.Mutex
	active, maxActive := 0, 0
	rec := &recorder{turns: [][]llm.Event{
		{done(twoEchoCalls())},
		{done(textAssistant("done"))},
	}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn(), ToolExecution: ExecutionSequential}
	base := Context{Tools: []AgentTool{overlapTool("", &active, &maxActive, &mu)}}

	collect(RunLoop(context.Background(), []AgentMessage{userPrompt("go")}, base, cfg))

	mu.Lock()
	defer mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("max concurrent tools = %d, want 1 (sequential mode)", maxActive)
	}
}

func TestExecuteToolCallsSequentialToolForcesBatch(t *testing.T) {
	var mu sync.Mutex
	active, maxActive := 0, 0
	rec := &recorder{turns: [][]llm.Event{
		{done(twoEchoCalls())},
		{done(textAssistant("done"))},
	}}
	// Default (parallel) loop, but the tool itself declares sequential, which
	// forces the whole batch sequential.
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}
	base := Context{Tools: []AgentTool{overlapTool(ExecutionSequential, &active, &maxActive, &mu)}}

	collect(RunLoop(context.Background(), []AgentMessage{userPrompt("go")}, base, cfg))

	mu.Lock()
	defer mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("max concurrent tools = %d, want 1 (sequential tool forces batch)", maxActive)
	}
}
