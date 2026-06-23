package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/llm"
)

// --- test helpers ---------------------------------------------------------

var testModel = llm.Model{ID: "m", Provider: "p", Protocol: llm.ProtocolOpenAICompletions}

type echoArgs struct {
	Text string `json:"text"`
}

// recorder is a scripted StreamFn that returns one prepared turn per call and
// records the model and input it was given.
type recorder struct {
	turns  [][]llm.Event
	calls  int
	models []llm.Model
	inputs []llm.Context
}

func (r *recorder) fn() StreamFn {
	return func(_ context.Context, model llm.Model, input llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		r.models = append(r.models, model)
		r.inputs = append(r.inputs, input)
		turn := r.turns[r.calls]
		r.calls++
		ch := make(chan llm.Event, len(turn))
		for _, event := range turn {
			ch <- event
		}
		close(ch)
		return ch, nil
	}
}

func userPrompt(text string) AgentMessage {
	return FromLLM(&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: text}}})
}

func textAssistant(text string) *llm.AssistantMessage {
	return &llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content:    []llm.AssistantContent{&llm.TextContent{Text: text}},
	}
}

func toolCallAssistant(id, name string, args map[string]any) *llm.AssistantMessage {
	return &llm.AssistantMessage{
		StopReason: llm.StopReasonToolUse,
		Content:    []llm.AssistantContent{&llm.ToolCall{ID: id, Name: name, Arguments: args}},
	}
}

func done(message *llm.AssistantMessage) llm.Event {
	return llm.Event{Type: llm.EventDone, Message: message}
}

func boolPtr(b bool) *bool { return &b }

func collect(ch <-chan AgentEvent) []AgentEvent {
	var events []AgentEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func eventTypes(events []AgentEvent) []AgentEventType {
	types := make([]AgentEventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func agentEndMessages(t *testing.T, events []AgentEvent) []AgentMessage {
	t.Helper()
	for _, event := range events {
		if event.Type == AgentEnd {
			return event.Messages
		}
	}
	t.Fatal("no agent_end event")
	return nil
}

func assistantOf(t *testing.T, message AgentMessage) *llm.AssistantMessage {
	t.Helper()
	wrapped, ok := message.(llmMessage)
	if !ok {
		t.Fatalf("not an llm message: %T", message)
	}
	assistant, ok := wrapped.Message.(*llm.AssistantMessage)
	if !ok {
		t.Fatalf("not an assistant message: %T", wrapped.Message)
	}
	return assistant
}

func toolResultOf(t *testing.T, message AgentMessage) *llm.ToolResultMessage {
	t.Helper()
	wrapped, ok := message.(llmMessage)
	if !ok {
		t.Fatalf("not an llm message: %T", message)
	}
	result, ok := wrapped.Message.(*llm.ToolResultMessage)
	if !ok {
		t.Fatalf("not a tool result message: %T", wrapped.Message)
	}
	return result
}

func resultText(content []llm.ToolResultContent) string {
	for _, block := range content {
		if text, ok := block.(*llm.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func echoTool(execute func()) AgentTool {
	return AgentTool{
		Definition: llm.MustTool[echoArgs]("echo", "Echo text back"),
		Execute: func(_ context.Context, _ string, args json.RawMessage, _ func(ToolResult)) (ToolResult, error) {
			if execute != nil {
				execute()
			}
			var parsed echoArgs
			_ = json.Unmarshal(args, &parsed)
			return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "echoed: " + parsed.Text}}}, nil
		},
	}
}

// --- tests ----------------------------------------------------------------

func TestRunLoopTextResponse(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{{done(textAssistant("hello"))}}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("hi")}, Context{}, cfg))

	want := []AgentEventType{
		AgentStart, TurnStart,
		MessageStart, MessageEnd, // prompt
		MessageStart, MessageEnd, // assistant
		TurnEnd, AgentEnd,
	}
	if got := eventTypes(events); !equalTypes(got, want) {
		t.Fatalf("event sequence:\n got %v\nwant %v", got, want)
	}

	messages := agentEndMessages(t, events)
	if len(messages) != 2 {
		t.Fatalf("appended messages = %d, want 2", len(messages))
	}
	if got := assistantOf(t, messages[1]); textOf(got) != "hello" {
		t.Fatalf("assistant text = %q, want %q", textOf(got), "hello")
	}
	if rec.calls != 1 {
		t.Fatalf("stream calls = %d, want 1", rec.calls)
	}
}

func TestRunLoopToolCallThenText(t *testing.T) {
	executed := 0
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{"text": "hi"}))},
		{done(textAssistant("done"))},
	}}
	cfg := LoopConfig{
		Model:    testModel,
		StreamFn: rec.fn(),
	}
	base := Context{Tools: []AgentTool{echoTool(func() { executed++ })}}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("use echo")}, base, cfg))

	if executed != 1 {
		t.Fatalf("tool executed %d times, want 1", executed)
	}
	if rec.calls != 2 {
		t.Fatalf("stream calls = %d, want 2", rec.calls)
	}
	if len(rec.inputs) != 2 {
		t.Fatalf("recorded inputs = %d, want 2", len(rec.inputs))
	}
	secondTurn := rec.inputs[1].Messages
	if len(secondTurn) != 3 {
		t.Fatalf("second-turn history = %d messages, want 3 (user, assistant tool call, tool result)", len(secondTurn))
	}
	if _, ok := secondTurn[0].(*llm.UserMessage); !ok {
		t.Fatalf("second-turn message 0 = %T, want *llm.UserMessage", secondTurn[0])
	}
	if _, ok := secondTurn[1].(*llm.AssistantMessage); !ok {
		t.Fatalf("second-turn message 1 = %T, want *llm.AssistantMessage", secondTurn[1])
	}
	if _, ok := secondTurn[2].(*llm.ToolResultMessage); !ok {
		t.Fatalf("second-turn message 2 = %T, want *llm.ToolResultMessage", secondTurn[2])
	}
	if !hasType(events, ToolStart) || !hasType(events, ToolEnd) {
		t.Fatal("missing tool execution events")
	}

	messages := agentEndMessages(t, events)
	if len(messages) != 4 {
		t.Fatalf("appended messages = %d, want 4 (prompt, assistant, result, assistant)", len(messages))
	}
	result := toolResultOf(t, messages[2])
	if result.IsError {
		t.Fatal("tool result marked as error")
	}
	if resultText(result.Content) != "echoed: hi" {
		t.Fatalf("tool result text = %q, want %q", resultText(result.Content), "echoed: hi")
	}
}

func TestRunLoopBeforeToolCallBlocks(t *testing.T) {
	executed := 0
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{"text": "hi"}))},
		{done(textAssistant("ok"))},
	}}
	cfg := LoopConfig{
		Model:    testModel,
		StreamFn: rec.fn(),
		BeforeToolCall: func(BeforeToolCallCtx) (bool, string) {
			return true, "not allowed"
		},
	}
	base := Context{Tools: []AgentTool{echoTool(func() { executed++ })}}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("use echo")}, base, cfg))

	if executed != 0 {
		t.Fatalf("blocked tool executed %d times, want 0", executed)
	}
	result := toolResultOf(t, agentEndMessages(t, events)[2])
	if !result.IsError {
		t.Fatal("blocked tool result should be an error")
	}
	if resultText(result.Content) != "not allowed" {
		t.Fatalf("blocked reason = %q, want %q", resultText(result.Content), "not allowed")
	}
}

func TestRunLoopAfterToolCallOverridesAndTerminates(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{"text": "hi"}))},
	}}
	cfg := LoopConfig{
		Model:    testModel,
		StreamFn: rec.fn(),
		AfterToolCall: func(AfterToolCallCtx) *AfterToolCallResult {
			return &AfterToolCallResult{
				Content:   []llm.ToolResultContent{&llm.TextContent{Text: "overridden"}},
				Terminate: boolPtr(true),
			}
		},
	}
	base := Context{Tools: []AgentTool{echoTool(nil)}}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("use echo")}, base, cfg))

	if rec.calls != 1 {
		t.Fatalf("stream calls = %d, want 1 (terminate should stop the loop)", rec.calls)
	}
	result := toolResultOf(t, agentEndMessages(t, events)[2])
	if resultText(result.Content) != "overridden" {
		t.Fatalf("overridden content = %q, want %q", resultText(result.Content), "overridden")
	}
}

func TestRunLoopToolTerminateStopsLoop(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "stop", nil))},
	}}
	stopTool := AgentTool{
		Definition: llm.MustTool[struct{}]("stop", "Stop the run"),
		Execute: func(_ context.Context, _ string, _ json.RawMessage, _ func(ToolResult)) (ToolResult, error) {
			return ToolResult{
				Content:   []llm.ToolResultContent{&llm.TextContent{Text: "stopping"}},
				Terminate: true,
			}, nil
		},
	}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}
	base := Context{Tools: []AgentTool{stopTool}}

	collect(RunLoop(context.Background(), []AgentMessage{userPrompt("stop")}, base, cfg))

	if rec.calls != 1 {
		t.Fatalf("stream calls = %d, want 1", rec.calls)
	}
}

func TestRunLoopErrorStopReasonEndsRun(t *testing.T) {
	failed := &llm.AssistantMessage{StopReason: llm.StopReasonError, ErrorMessage: "boom"}
	rec := &recorder{turns: [][]llm.Event{
		{{Type: llm.EventError, Message: failed}},
		{done(textAssistant("unreached"))},
	}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("hi")}, Context{}, cfg))

	if rec.calls != 1 {
		t.Fatalf("stream calls = %d, want 1 (error should end the run)", rec.calls)
	}
	if events[len(events)-1].Type != AgentEnd {
		t.Fatalf("last event = %q, want agent_end", events[len(events)-1].Type)
	}
}

func TestRunLoopPrepareNextTurnSwitchesModel(t *testing.T) {
	second := llm.Model{ID: "m2", Provider: "p2", Protocol: llm.ProtocolAnthropicMessages}
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{"text": "hi"}))},
		{done(textAssistant("done"))},
	}}
	cfg := LoopConfig{
		Model:    testModel,
		StreamFn: rec.fn(),
		PrepareNextTurn: func(TurnCtx) *TurnUpdate {
			return &TurnUpdate{Model: &second}
		},
	}
	base := Context{Tools: []AgentTool{echoTool(nil)}}

	collect(RunLoop(context.Background(), []AgentMessage{userPrompt("use echo")}, base, cfg))

	if len(rec.models) != 2 {
		t.Fatalf("stream calls = %d, want 2", len(rec.models))
	}
	if rec.models[0].ID != "m" || rec.models[1].ID != "m2" {
		t.Fatalf("models used = [%s %s], want [m m2]", rec.models[0].ID, rec.models[1].ID)
	}
}

func TestRunLoopFollowUpContinuesRun(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{
		{done(textAssistant("first"))},
		{done(textAssistant("second"))},
	}}
	followUps := 0
	cfg := LoopConfig{
		Model:    testModel,
		StreamFn: rec.fn(),
		GetFollowUpMessages: func() []AgentMessage {
			followUps++
			if followUps == 1 {
				return []AgentMessage{userPrompt("more")}
			}
			return nil
		},
	}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("hi")}, Context{}, cfg))

	if rec.calls != 2 {
		t.Fatalf("stream calls = %d, want 2", rec.calls)
	}
	messages := agentEndMessages(t, events)
	if len(messages) != 4 {
		t.Fatalf("appended messages = %d, want 4 (prompt, first, follow-up, second)", len(messages))
	}
}

func TestDefaultConvertToLLMFiltersCustom(t *testing.T) {
	type note struct {
		Custom
		Text string
	}
	messages := []AgentMessage{userPrompt("hi"), note{Text: "ignored"}}

	converted := defaultConvertToLLM(messages)

	if len(converted) != 1 {
		t.Fatalf("converted = %d messages, want 1 (custom dropped)", len(converted))
	}
	if _, ok := converted[0].(*llm.UserMessage); !ok {
		t.Fatalf("converted[0] = %T, want *llm.UserMessage", converted[0])
	}
}

func TestRunLoopRecoversFromCallbackPanic(t *testing.T) {
	rec := &recorder{turns: [][]llm.Event{{done(textAssistant("hi"))}}}
	cfg := LoopConfig{
		Model:    testModel,
		StreamFn: rec.fn(),
		PrepareNextTurn: func(TurnCtx) *TurnUpdate {
			panic("boom")
		},
	}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("hi")}, Context{}, cfg))

	if last := events[len(events)-1]; last.Type != AgentEnd {
		t.Fatalf("last event = %q, want agent_end (a panicking hook must not crash the loop)", last.Type)
	}
	messages := agentEndMessages(t, events)
	if len(messages) != 1 {
		t.Fatalf("agent_end messages = %d, want 1 (the error message)", len(messages))
	}
	if failed := assistantOf(t, messages[0]); failed.StopReason != llm.StopReasonError {
		t.Fatalf("stop reason = %q, want error", failed.StopReason)
	}
}

func TestRunLoopRecoversFromToolPanic(t *testing.T) {
	panicTool := AgentTool{
		Definition: llm.MustTool[echoArgs]("echo", "echo"),
		Execute: func(context.Context, string, json.RawMessage, func(ToolResult)) (ToolResult, error) {
			panic("tool boom")
		},
	}
	rec := &recorder{turns: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{"text": "x"}))},
		{done(textAssistant("recovered"))},
	}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}
	base := Context{Tools: []AgentTool{panicTool}}

	events := collect(RunLoop(context.Background(), []AgentMessage{userPrompt("go")}, base, cfg))

	result := toolResultOf(t, agentEndMessages(t, events)[2])
	if !result.IsError {
		t.Fatal("a panicking tool should produce an error result")
	}
	if rec.calls != 2 {
		t.Fatalf("stream calls = %d, want 2 (the run should continue past a tool panic)", rec.calls)
	}
}

func TestRunLoopSequentialAbortSkipsRemainingTools(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	secondRan := false
	tool := AgentTool{
		Definition:    llm.MustTool[echoArgs]("echo", "echo"),
		ExecutionMode: ExecutionSequential,
		Execute: func(_ context.Context, callID string, _ json.RawMessage, _ func(ToolResult)) (ToolResult, error) {
			if callID == "a" {
				cancel() // cancel the run after the first tool runs
			}
			if callID == "b" {
				secondRan = true
			}
			return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "ok"}}}, nil
		},
	}
	rec := &recorder{turns: [][]llm.Event{
		{done(twoEchoCalls())}, // tool calls "a" then "b"
		{{Type: llm.EventError, Message: &llm.AssistantMessage{StopReason: llm.StopReasonAborted, ErrorMessage: "aborted"}}},
	}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}
	base := Context{Tools: []AgentTool{tool}}

	collect(RunLoop(ctx, []AgentMessage{userPrompt("go")}, base, cfg))

	if secondRan {
		t.Fatal("second sequential tool ran after the run was cancelled, want it skipped")
	}
}

// --- small local helpers --------------------------------------------------

func equalTypes(a, b []AgentEventType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasType(events []AgentEvent, target AgentEventType) bool {
	for _, event := range events {
		if event.Type == target {
			return true
		}
	}
	return false
}

func textOf(message *llm.AssistantMessage) string {
	for _, block := range message.Content {
		if text, ok := block.(*llm.TextContent); ok {
			return text.Text
		}
	}
	return ""
}
