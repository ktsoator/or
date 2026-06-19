package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ktsoator/or/internal/llm"
)

func TestStreamAggregatesThinkingToolCallAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		anthropicSSE(w, "message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"test-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":2,"cache_read_input_tokens":3}}}`)
		anthropicSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`)
		anthropicSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"plan "}}`)
		anthropicSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_1"}}`)
		anthropicSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		anthropicSSE(w, "content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"weather","input":{}}}`)
		anthropicSSE(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`)
		anthropicSSE(w, "content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}`)
		anthropicSSE(w, "content_block_stop", `{"type":"content_block_stop","index":1}`)
		anthropicSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":6}}`)
		anthropicSSE(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	events := streamAnthropicTest(t, context.Background(), server.URL)
	assertAnthropicSingleTerminal(t, events, llm.EventDone)
	assertAnthropicEventTypes(t, events, []llm.EventType{
		llm.EventStart,
		llm.EventThinkingStart,
		llm.EventThinkingDelta,
		llm.EventThinkingEnd,
		llm.EventToolCallStart,
		llm.EventToolCallDelta,
		llm.EventToolCallDelta,
		llm.EventToolCallEnd,
		llm.EventDone,
	})

	message := events[len(events)-1].Message
	if message == nil {
		t.Fatal("done event has no message")
	}
	if message.ResponseID != "msg_1" || message.StopReason != llm.StopReasonToolUse {
		t.Fatalf("message metadata = %#v", message)
	}
	thinking, ok := message.Content[0].(*llm.ThinkingContent)
	if !ok || thinking.Thinking != "plan " || thinking.ThinkingSignature != "sig_1" {
		t.Fatalf("thinking content = %#v", message.Content[0])
	}
	call, ok := message.Content[1].(*llm.ToolCall)
	if !ok || call.ID != "toolu_1" || call.Name != "weather" || call.Arguments["city"] != "Paris" {
		t.Fatalf("tool call = %#v", message.Content[1])
	}
	if got := message.Usage; got.Input != 10 || got.Output != 6 || got.CacheRead != 3 || got.CacheWrite != 2 || got.TotalTokens != 21 {
		t.Fatalf("usage = %#v", got)
	}
}

func TestStreamCancellationEmitsOneAbortedTerminalEvent(t *testing.T) {
	requestDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestDone)
		w.Header().Set("Content-Type", "text/event-stream")
		anthropicSSE(w, "message_start", `{"type":"message_start","message":{"id":"msg_cancel","type":"message","role":"assistant","content":[],"model":"test-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := NewAdapter(server.Client()).Stream(ctx, anthropicTestModel(server.URL), anthropicTestContext(), llm.StreamOptions{APIKey: "test"})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var events []llm.Event
	for event := range stream {
		events = append(events, event)
		if event.Type == llm.EventStart {
			cancel()
		}
	}
	assertAnthropicSingleTerminal(t, events, llm.EventError)
	terminal := events[len(events)-1]
	if terminal.Message == nil || terminal.Message.StopReason != llm.StopReasonAborted {
		t.Fatalf("terminal message = %#v", terminal.Message)
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request context was not cancelled")
	}
}

func anthropicSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func streamAnthropicTest(t *testing.T, ctx context.Context, baseURL string) []llm.Event {
	t.Helper()
	stream, err := NewAdapter(nil).Stream(ctx, anthropicTestModel(baseURL), anthropicTestContext(), llm.StreamOptions{APIKey: "test"})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var events []llm.Event
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func anthropicTestModel(baseURL string) llm.Model {
	return llm.Model{
		ID:        "test-model",
		Protocol:  llm.ProtocolAnthropicMessages,
		Provider:  "test",
		BaseURL:   baseURL,
		Input:     []llm.ModelInput{llm.Text},
		Cost:      llm.ModelCost{Input: 1, Output: 2, CacheRead: 0.5, CacheWrite: 1.5},
		MaxTokens: 128,
	}
}

func anthropicTestContext() llm.Context {
	return llm.Context{Messages: []llm.Message{&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: "hello"}},
	}}}
}

func assertAnthropicEventTypes(t *testing.T, events []llm.Event, want []llm.EventType) {
	t.Helper()
	got := make([]llm.EventType, len(events))
	for i := range events {
		got[i] = events[i].Type
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("event types = %v, want %v", got, want)
	}
}

func assertAnthropicSingleTerminal(t *testing.T, events []llm.Event, want llm.EventType) {
	t.Helper()
	terminals := 0
	for _, event := range events {
		if event.Type == llm.EventDone || event.Type == llm.EventError {
			terminals++
			if event.Type != want {
				t.Fatalf("terminal event = %q, want %q", event.Type, want)
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal event count = %d, want 1", terminals)
	}
	if len(events) == 0 || events[len(events)-1].Type != want {
		t.Fatalf("last event = %#v, want terminal %q", events, want)
	}
}
