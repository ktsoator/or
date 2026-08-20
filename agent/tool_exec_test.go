package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ktsoator/or/llm"
)

// twoEchoCalls is an assistant step that calls the echo tool twice.
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
		Execute: func(_ context.Context, _ string, args json.RawMessage, _ func(ToolProgress)) (ToolResult, error) {
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
		Execute: func(_ context.Context, _ string, args json.RawMessage, _ func(ToolProgress)) (ToolResult, error) {
			started.Done()
			<-release
			var parsed echoArgs
			_ = json.Unmarshal(args, &parsed)
			return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "echoed: " + parsed.Text}}}, nil
		},
	}
	rec := &recorder{steps: [][]llm.Event{
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

func TestToolProgressPrecedesOneTerminalOutcomeAndLateUpdatesAreIgnored(t *testing.T) {
	var lateProgress func(ToolProgress)
	afterCalls := 0
	tool := AgentTool{
		Definition: llm.MustTool[echoArgs]("echo", "echo"),
		Execute: func(_ context.Context, _ string, _ json.RawMessage, onProgress func(ToolProgress)) (ToolResult, error) {
			onProgress(ToolProgress{Data: "working"})
			lateProgress = onProgress
			return ToolResult{Outcome: ToolOutcome{
				Status:    ToolOutcomeFailed,
				ErrorCode: "expected_failure",
				Data:      "terminal",
			}}, nil
		},
	}
	var events []AgentEvent
	engine := engine{
		ctx: context.Background(),
		cfg: LoopConfig{AfterToolCall: func(ctx AfterToolCallCtx) *AfterToolCallResult {
			afterCalls++
			if ctx.Result.Outcome.Data != "terminal" {
				t.Fatalf("hook outcome data = %#v, want terminal", ctx.Result.Outcome.Data)
			}
			return nil
		}},
		emit: func(event AgentEvent) { events = append(events, event) },
	}
	engine.runTool(
		Context{Tools: []AgentTool{tool}},
		llm.AssistantMessage{},
		llm.ToolCall{ID: "call-1", Name: "echo", Arguments: map[string]any{"text": "hi"}},
	)
	lateProgress(ToolProgress{Data: "too late"})

	wantTypes := []AgentEventType{ToolStart, ToolUpdate, ToolEnd, MessageStart, MessageEnd}
	if got := eventTypes(events); !equalTypes(got, wantTypes) {
		t.Fatalf("event types = %v, want %v", got, wantTypes)
	}
	if events[1].Progress.Data != "working" {
		t.Fatalf("progress = %#v, want working", events[1].Progress)
	}
	if events[2].Result.Outcome.Status != ToolOutcomeFailed ||
		events[2].Result.Outcome.ErrorCode != "expected_failure" ||
		events[2].Result.Outcome.Data != "terminal" {
		t.Fatalf("terminal result = %#v", events[2].Result)
	}
	if afterCalls != 1 {
		t.Fatalf("AfterToolCall calls = %d, want 1", afterCalls)
	}
}

func TestToolFailuresEmitExactlyOneTerminalEvent(t *testing.T) {
	tests := []struct {
		name       string
		execute    func(func(ToolProgress)) (ToolResult, error)
		wantStatus ToolOutcomeStatus
	}{
		{
			name: "error",
			execute: func(onProgress func(ToolProgress)) (ToolResult, error) {
				onProgress(ToolProgress{Data: "started"})
				return ToolResult{}, errors.New("boom")
			},
			wantStatus: ToolOutcomeFailed,
		},
		{
			name: "cancelled",
			execute: func(onProgress func(ToolProgress)) (ToolResult, error) {
				onProgress(ToolProgress{Data: "started"})
				return ToolResult{}, context.Canceled
			},
			wantStatus: ToolOutcomeCancelled,
		},
		{
			name: "panic",
			execute: func(onProgress func(ToolProgress)) (ToolResult, error) {
				onProgress(ToolProgress{Data: "started"})
				panic("boom")
			},
			wantStatus: ToolOutcomeFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := AgentTool{
				Definition: llm.MustTool[echoArgs]("echo", "echo"),
				Execute: func(_ context.Context, _ string, _ json.RawMessage, onProgress func(ToolProgress)) (ToolResult, error) {
					return test.execute(onProgress)
				},
			}
			var events []AgentEvent
			engine := engine{ctx: context.Background(), emit: func(event AgentEvent) {
				events = append(events, event)
			}}
			engine.runTool(
				Context{Tools: []AgentTool{tool}},
				llm.AssistantMessage{},
				llm.ToolCall{ID: "call-1", Name: "echo", Arguments: map[string]any{"text": "hi"}},
			)

			terminal := 0
			for _, event := range events {
				if event.Type != ToolEnd {
					continue
				}
				terminal++
				if event.Result.Outcome.Status != test.wantStatus {
					t.Fatalf("terminal status = %q, want %q", event.Result.Outcome.Status, test.wantStatus)
				}
			}
			if terminal != 1 {
				t.Fatalf("ToolEnd count = %d, want 1; events=%v", terminal, eventTypes(events))
			}
		})
	}
}

func TestParallelToolProgressMayInterleaveButTerminalOrderIsStable(t *testing.T) {
	var started sync.WaitGroup
	started.Add(2)
	release := make(chan struct{})
	go func() {
		started.Wait()
		close(release)
	}()
	tool := AgentTool{
		Definition: llm.MustTool[echoArgs]("echo", "echo"),
		Execute: func(_ context.Context, callID string, _ json.RawMessage, onProgress func(ToolProgress)) (ToolResult, error) {
			started.Done()
			<-release
			onProgress(ToolProgress{Data: callID})
			return ToolResult{}, nil
		},
	}
	var mu sync.Mutex
	var events []AgentEvent
	engine := engine{ctx: context.Background(), emit: func(event AgentEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}}
	engine.executeToolCalls(Context{Tools: []AgentTool{tool}}, *twoEchoCalls(), []llm.ToolCall{
		{ID: "a", Name: "echo", Arguments: map[string]any{"text": "1"}},
		{ID: "b", Name: "echo", Arguments: map[string]any{"text": "2"}},
	})

	firstTerminal := -1
	var progressIDs []string
	var terminalIDs []string
	for index, event := range events {
		switch event.Type {
		case ToolUpdate:
			progressIDs = append(progressIDs, event.Progress.Data.(string))
			if firstTerminal >= 0 {
				t.Fatalf("progress emitted after ToolEnd: events=%v", eventTypes(events))
			}
		case ToolEnd:
			if firstTerminal < 0 {
				firstTerminal = index
			}
			terminalIDs = append(terminalIDs, event.ToolCallID)
		}
	}
	if len(progressIDs) != 2 {
		t.Fatalf("progress ids = %v, want both calls", progressIDs)
	}
	if len(terminalIDs) != 2 || terminalIDs[0] != "a" || terminalIDs[1] != "b" {
		t.Fatalf("terminal ids = %v, want [a b]", terminalIDs)
	}
}

func TestExecuteToolCallsSequentialModeDoesNotOverlap(t *testing.T) {
	var mu sync.Mutex
	active, maxActive := 0, 0
	rec := &recorder{steps: [][]llm.Event{
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
	rec := &recorder{steps: [][]llm.Event{
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

func TestExecutePreparedNormalizesToolOutcomes(t *testing.T) {
	exitCode := 9
	tests := []struct {
		name      string
		result    ToolResult
		err       error
		want      ToolOutcome
		wantError bool
	}{
		{
			name:   "zero value is success",
			result: ToolResult{Outcome: ToolOutcome{Data: "structured"}},
			want:   ToolOutcome{Status: ToolOutcomeSuccess, Data: "structured"},
		},
		{
			name: "explicit failure",
			result: ToolResult{Outcome: ToolOutcome{
				Status:    ToolOutcomeFailed,
				ErrorCode: "command_exit_nonzero",
				ExitCode:  &exitCode,
				Data:      "diagnostic",
			}},
			want: ToolOutcome{
				Status:    ToolOutcomeFailed,
				ErrorCode: "command_exit_nonzero",
				ExitCode:  &exitCode,
				Data:      "diagnostic",
			},
			wantError: true,
		},
		{
			name:      "failure gets a stable default code",
			result:    ToolResult{Outcome: ToolOutcome{Status: ToolOutcomeFailed}},
			want:      ToolOutcome{Status: ToolOutcomeFailed, ErrorCode: "tool_failed"},
			wantError: true,
		},
		{
			name:      "deadline error is timeout",
			err:       context.DeadlineExceeded,
			want:      ToolOutcome{Status: ToolOutcomeTimeout, ErrorCode: "tool_execution_timeout"},
			wantError: true,
		},
		{
			name:      "cancel error is cancelled",
			err:       context.Canceled,
			want:      ToolOutcome{Status: ToolOutcomeCancelled, ErrorCode: "tool_execution_cancelled"},
			wantError: true,
		},
		{
			name:      "ordinary error is failed",
			err:       errors.New("boom"),
			want:      ToolOutcome{Status: ToolOutcomeFailed, ErrorCode: "tool_execution_failed"},
			wantError: true,
		},
		{
			name:      "unknown status is failed",
			result:    ToolResult{Outcome: ToolOutcome{Status: "unexpected"}},
			want:      ToolOutcome{Status: ToolOutcomeFailed, ErrorCode: "invalid_tool_outcome"},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := AgentTool{Execute: func(context.Context, string, json.RawMessage, func(ToolProgress)) (ToolResult, error) {
				return test.result, test.err
			}}
			engine := engine{ctx: context.Background(), emit: func(AgentEvent) {}}
			got := engine.executePrepared(preparedToolCall{
				call: llm.ToolCall{ID: "call-1", Name: "test"},
				tool: &tool,
			})
			if got.Outcome.Status != test.want.Status ||
				got.Outcome.ErrorCode != test.want.ErrorCode ||
				got.Outcome.Data != test.want.Data ||
				!sameOptionalInt(got.Outcome.ExitCode, test.want.ExitCode) {
				t.Fatalf("outcome = %#v, want %#v", got.Outcome, test.want)
			}
			if got.Outcome.Failed() != test.wantError {
				t.Fatalf("outcome failed = %v, want %v", got.Outcome.Failed(), test.wantError)
			}
		})
	}
}

func TestFinishProjectsOutcomeFailureToModelAndEvents(t *testing.T) {
	var events []AgentEvent
	engine := engine{
		emit: func(event AgentEvent) { events = append(events, event) },
	}
	outcome := ToolOutcome{Status: ToolOutcomeTimeout, ErrorCode: "browser_navigation_timeout"}
	message, _ := engine.finish(
		llm.ToolCall{ID: "call-1", Name: "open_preview"},
		ToolResult{Outcome: outcome},
	)

	if !message.IsError {
		t.Fatal("timeout outcome was not projected as an LLM tool error")
	}
	if len(events) != 3 || events[0].Type != ToolEnd || !events[0].IsError {
		t.Fatalf("events = %#v, want an error ToolEnd followed by message events", events)
	}
	result := events[0].Result
	if result.Outcome != outcome {
		t.Fatalf("tool end result = %#v, want outcome %#v", events[0].Result, outcome)
	}
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
