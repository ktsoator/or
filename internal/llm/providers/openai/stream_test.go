package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/internal/llm"
)

func TestStreamAggregatesThinkingToolCallAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"reasoning_content":"plan "},"finish_reason":null}]}`,
			`{"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Paris\"}"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":3}}}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	events := streamOpenAITest(t, server.URL+"/v1")
	assertSingleTerminalEvent(t, events, llm.EventDone)
	assertEventTypes(t, events, []llm.EventType{
		llm.EventStart,
		llm.EventThinkingStart,
		llm.EventThinkingDelta,
		llm.EventToolCallStart,
		llm.EventToolCallDelta,
		llm.EventToolCallDelta,
		llm.EventThinkingEnd,
		llm.EventToolCallEnd,
		llm.EventDone,
	})

	message := events[len(events)-1].Message
	if message == nil {
		t.Fatal("done event has no message")
	}
	if message.StopReason != llm.StopReasonToolUse {
		t.Fatalf("stop reason = %q, want %q", message.StopReason, llm.StopReasonToolUse)
	}
	thinking, ok := message.Content[0].(*llm.ThinkingContent)
	if !ok || thinking.Thinking != "plan " {
		t.Fatalf("thinking content = %#v, want plan", message.Content[0])
	}
	call, ok := message.Content[1].(*llm.ToolCall)
	if !ok {
		t.Fatalf("tool content type = %T", message.Content[1])
	}
	if call.ID != "call_1" || call.Name != "weather" || call.Arguments["city"] != "Paris" {
		t.Fatalf("tool call = %#v", call)
	}
	if got := message.Usage; got.Input != 7 || got.Output != 5 || got.CacheRead != 3 || got.TotalTokens != 15 {
		t.Fatalf("usage = %#v", got)
	}
}

func TestStreamCancellationEmitsOneAbortedTerminalEvent(t *testing.T) {
	requestDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestDone)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl_cancel\",\"object\":\"chat.completion.chunk\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"started\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := NewAdapter(server.Client()).Stream(ctx, openAITestModel(server.URL+"/v1"), openAITestContext(), llm.StreamOptions{APIKey: "test"})
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
	assertSingleTerminalEvent(t, events, llm.EventError)
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

func TestStreamMalformedToolArgumentsEmitsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"id":"chatcmpl_bad","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_bad","type":"function","function":{"name":"weather","arguments":"{\"city\":"}}]},"finish_reason":null}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"id":"chatcmpl_bad","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "data: [DONE]")
		fmt.Fprintln(w)
	}))
	defer server.Close()

	events := streamOpenAITest(t, server.URL+"/v1")
	assertSingleTerminalEvent(t, events, llm.EventError)
	terminal := events[len(events)-1]
	if terminal.Err == nil || !strings.Contains(terminal.Err.Error(), "parse arguments for tool call") {
		t.Fatalf("terminal error = %v", terminal.Err)
	}
	if terminal.Message == nil || terminal.Message.StopReason != llm.StopReasonError {
		t.Fatalf("terminal message = %#v", terminal.Message)
	}
}

func streamOpenAITest(t *testing.T, baseURL string) []llm.Event {
	t.Helper()
	stream, err := NewAdapter(nil).Stream(
		context.Background(),
		openAITestModel(baseURL),
		openAITestContext(),
		llm.StreamOptions{APIKey: "test"},
	)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var events []llm.Event
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func openAITestModel(baseURL string) llm.Model {
	return llm.Model{
		ID:       "test-model",
		Protocol: llm.ProtocolOpenAICompletions,
		Provider: "test",
		BaseURL:  baseURL,
		Input:    []llm.ModelInput{llm.Text},
		Cost:     llm.ModelCost{Input: 1, Output: 2, CacheRead: 0.5},
	}
}

func openAITestContext() llm.Context {
	return llm.Context{Messages: []llm.Message{&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: "hello"}},
	}}}
}

func assertEventTypes(t *testing.T, events []llm.Event, want []llm.EventType) {
	t.Helper()
	got := make([]llm.EventType, len(events))
	for i := range events {
		got[i] = events[i].Type
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("event types = %v, want %v", got, want)
	}
}

func assertSingleTerminalEvent(t *testing.T, events []llm.Event, want llm.EventType) {
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
