package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/ktsoator/or/internal/llm"
)

func TestOpenAIProviderStreamsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer test-key" {
			t.Errorf("unexpected authorization header: %q", authorization)
		}

		var request struct {
			Model         string `json:"model"`
			StreamOptions struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "test-model" {
			t.Errorf("unexpected model: %q", request.Model)
		}
		if !request.StreamOptions.IncludeUsage {
			t.Error("stream usage was not requested")
		}
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Errorf("unexpected messages: %#v", request.Messages)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"response-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"think \"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"response-model\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"carefully\",\"content\":\"hello \"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"response-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"world\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"response-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":5,\"total_tokens\":17,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	registry := llm.NewRegistry()
	if err := registry.Register(NewAdapter(server.Client())); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	events, err := llm.NewClient(registry).Stream(
		context.Background(),
		llm.Model{
			ID:       "test-model",
			Protocol: llm.ProtocolOpenAICompletions,
			Provider: "openai",
			BaseURL:  server.URL + "/v1",
		},
		llm.Context{
			SystemPrompt: "You are helpful.",
			Messages: []llm.Message{&llm.UserMessage{
				Content: []llm.UserContent{&llm.TextContent{Text: "Say hello."}},
			}},
		},
		llm.StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var deltas string
	var thinkingDeltas string
	var eventTypes []llm.EventType
	var message *llm.AssistantMessage
	for event := range events {
		eventTypes = append(eventTypes, event.Type)
		switch event.Type {
		case llm.EventTextStart:
			if event.Partial == nil || event.ContentIndex >= len(event.Partial.Content) {
				t.Fatalf("unexpected text start partial: %#v", event.Partial)
			}
			content, ok := event.Partial.Content[event.ContentIndex].(*llm.TextContent)
			if !ok || content.Text != "" {
				t.Fatalf("unexpected text start partial: %#v", event.Partial)
			}
		case llm.EventTextDelta:
			if event.ContentIndex != 1 {
				t.Fatalf("text content index = %d, want 1", event.ContentIndex)
			}
			deltas += event.Delta
		case llm.EventThinkingDelta:
			if event.ContentIndex != 0 {
				t.Fatalf("thinking content index = %d, want 0", event.ContentIndex)
			}
			thinkingDeltas += event.Delta
		case llm.EventThinkingStart:
			if event.Partial == nil || event.ContentIndex >= len(event.Partial.Content) {
				t.Fatalf("unexpected thinking start partial: %#v", event.Partial)
			}
			content, ok := event.Partial.Content[event.ContentIndex].(*llm.ThinkingContent)
			if !ok || content.Thinking != "" {
				t.Fatalf("unexpected thinking start partial: %#v", event.Partial)
			}
		case llm.EventTextEnd:
			if event.ContentIndex != 1 || event.Content != "hello world" {
				t.Fatalf("unexpected text end event: %#v", event)
			}
		case llm.EventThinkingEnd:
			if event.ContentIndex != 0 || event.Content != "think carefully" {
				t.Fatalf("unexpected thinking end event: %#v", event)
			}
		case llm.EventDone:
			message = event.Message
		case llm.EventError:
			t.Fatalf("stream error: %v", event.Err)
		}
	}
	if deltas != "hello world" {
		t.Fatalf("unexpected text deltas: %q", deltas)
	}
	if thinkingDeltas != "think carefully" {
		t.Fatalf("unexpected thinking deltas: %q", thinkingDeltas)
	}
	wantEventTypes := []llm.EventType{
		llm.EventStart,
		llm.EventThinkingStart,
		llm.EventThinkingDelta,
		llm.EventThinkingDelta,
		llm.EventTextStart,
		llm.EventTextDelta,
		llm.EventTextDelta,
		llm.EventThinkingEnd,
		llm.EventTextEnd,
		llm.EventDone,
	}
	if !slices.Equal(eventTypes, wantEventTypes) {
		t.Fatalf("event types = %v, want %v", eventTypes, wantEventTypes)
	}
	if message == nil {
		t.Fatal("stream did not emit a final message")
	}
	if message.StopReason != "stop" {
		t.Fatalf("unexpected stop reason: %q", message.StopReason)
	}
	if message.Protocol != llm.ProtocolOpenAICompletions || message.Provider != "openai" || message.Model != "test-model" {
		t.Fatalf("unexpected response identity: %#v", message)
	}
	if message.ResponseID != "chatcmpl-1" || message.ResponseModel != "response-model" {
		t.Fatalf("unexpected provider response metadata: %#v", message)
	}
	if message.Timestamp == 0 {
		t.Fatal("response timestamp was not set")
	}
	if message.Usage.Input != 10 || message.Usage.Output != 5 || message.Usage.CacheRead != 2 || message.Usage.TotalTokens != 17 {
		t.Fatalf("unexpected usage: %#v", message.Usage)
	}
	if len(message.Content) != 2 {
		t.Fatalf("unexpected response content: %#v", message.Content)
	}
	thinking, thinkingOK := message.Content[0].(*llm.ThinkingContent)
	text, textOK := message.Content[1].(*llm.TextContent)
	if !thinkingOK || thinking.Thinking != "think carefully" || !textOK || text.Text != "hello world" {
		t.Fatalf("unexpected response content: %#v", message.Content)
	}
}

func TestConvertUserMessagePreservesContentParts(t *testing.T) {
	message, err := convertUserMessage(&llm.UserMessage{
		Content: []llm.UserContent{
			&llm.TextContent{Text: "Hello "},
			&llm.TextContent{Text: "world"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if message == nil || message.OfUser == nil {
		t.Fatal("expected a user message")
	}

	parts := message.OfUser.Content.OfArrayOfContentParts
	if len(parts) != 2 {
		t.Fatalf("content parts = %d, want 2", len(parts))
	}
	if text := parts[0].GetText(); text == nil || *text != "Hello " {
		t.Fatalf("first content part = %v, want %q", text, "Hello ")
	}
	if text := parts[1].GetText(); text == nil || *text != "world" {
		t.Fatalf("second content part = %v, want %q", text, "world")
	}
}

func TestConvertUserMessagePreservesImageContent(t *testing.T) {
	message, err := convertUserMessage(&llm.UserMessage{
		Content: []llm.UserContent{
			&llm.TextContent{Text: "What is this?"},
			&llm.ImageContent{MIMEType: "image/png", Data: "aW1hZ2U="},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	parts := message.OfUser.Content.OfArrayOfContentParts
	if len(parts) != 2 || parts[1].OfImageURL == nil {
		t.Fatalf("unexpected content parts: %#v", parts)
	}
	if url := parts[1].OfImageURL.ImageURL.URL; url != "data:image/png;base64,aW1hZ2U=" {
		t.Fatalf("image URL = %q", url)
	}
}

func TestConvertMessagesAttachesToolResultImages(t *testing.T) {
	messages, err := convertMessages(llm.Context{Messages: []llm.Message{
		&llm.ToolResultMessage{
			ToolCallID: "call_1",
			Content: []llm.ToolResultContent{
				&llm.ImageContent{MIMEType: "image/jpeg", Data: "aW1hZ2U="},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].OfTool == nil || messages[1].OfUser == nil {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if content := messages[0].OfTool.Content.OfString; content.Value != "(see attached image)" {
		t.Fatalf("tool result content = %q", content.Value)
	}
	parts := messages[1].OfUser.Content.OfArrayOfContentParts
	if len(parts) != 2 || parts[1].OfImageURL == nil {
		t.Fatalf("unexpected image attachment: %#v", parts)
	}
}

func TestOpenAIProviderStreamsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Type     string `json:"type"`
				Function struct {
					Name        string         `json:"name"`
					Description string         `json:"description"`
					Parameters  map[string]any `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
			Messages []struct {
				Role             string `json:"role"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCallID       string `json:"tool_call_id"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}

		if len(request.Tools) != 1 || request.Tools[0].Function.Name != "get_weather" {
			t.Errorf("unexpected tools: %#v", request.Tools)
		}

		var foundToolCall, foundToolResult bool
		for _, message := range request.Messages {
			if message.Role == "assistant" && len(message.ToolCalls) == 1 {
				if message.ReasoningContent != "I should call get_weather." {
					t.Errorf("unexpected assistant reasoning content: %q", message.ReasoningContent)
				}
				call := message.ToolCalls[0]
				if call.ID != "call_1" || call.Function.Name != "get_weather" || call.Function.Arguments != `{"city":"Paris"}` {
					t.Errorf("unexpected assistant tool call: %#v", call)
				}
				foundToolCall = true
			}
			if message.Role == "tool" && message.ToolCallID == "call_1" {
				foundToolResult = true
			}
		}
		if !foundToolCall {
			t.Errorf("request is missing the replayed assistant tool call")
		}
		if !foundToolResult {
			t.Errorf("request is missing the replayed tool result")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"{\\\"city\\\":\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"Paris\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	adapter := NewAdapter(server.Client())
	events, err := adapter.Stream(
		context.Background(),
		llm.Model{ID: "test-model", Protocol: llm.ProtocolOpenAICompletions, Provider: "openai", BaseURL: server.URL + "/v1"},
		llm.Context{
			Tools: []llm.ToolDefinition{{
				Name:        "get_weather",
				Description: "Get the weather for a city",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
			}},
			Messages: []llm.Message{
				&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "Weather in Paris?"}}},
				&llm.AssistantMessage{Content: []llm.AssistantContent{
					&llm.ThinkingContent{Thinking: "I should call get_weather."},
					&llm.ToolCall{
						ID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`,
					},
				}},
				&llm.ToolResultMessage{ToolCallID: "call_1", ToolName: "get_weather", Content: []llm.ToolResultContent{
					&llm.TextContent{Text: "Sunny, 20C"},
				}},
			},
		},
		llm.StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var ended *llm.ToolCall
	var started *llm.ToolCall
	var message *llm.AssistantMessage
	for event := range events {
		switch event.Type {
		case llm.EventToolCallStart:
			started = event.ToolCall
			if event.ContentIndex != 0 {
				t.Fatalf("tool call start content index = %d, want 0", event.ContentIndex)
			}
		case llm.EventToolCallEnd:
			ended = event.ToolCall
			if event.ContentIndex != 0 {
				t.Fatalf("tool call end content index = %d, want 0", event.ContentIndex)
			}
		case llm.EventDone:
			message = event.Message
		case llm.EventError:
			t.Fatalf("stream error: %v", event.Err)
		}
	}

	if started == nil || ended == nil {
		t.Fatal("stream did not emit a tool call end event")
	}
	if started.Arguments != "" {
		t.Fatalf("tool call start contains arguments: %#v", started)
	}
	if ended.ID != "call_2" || ended.Name != "get_weather" || ended.Arguments != `{"city":"Paris"}` {
		t.Fatalf("unexpected completed tool call: %#v", ended)
	}
	if message == nil {
		t.Fatal("stream did not emit a final message")
	}
	if message.StopReason != "toolUse" {
		t.Fatalf("unexpected stop reason: %q", message.StopReason)
	}
	if len(message.Content) != 1 {
		t.Fatalf("unexpected response content: %#v", message.Content)
	}
	call, ok := message.Content[0].(*llm.ToolCall)
	if !ok || call == nil {
		t.Fatalf("unexpected response content: %#v", message.Content)
	}
	if call.ID != "call_2" || call.Arguments != `{"city":"Paris"}` {
		t.Fatalf("unexpected tool call in final message: %#v", call)
	}
}

func TestOpenAIProviderCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	adapter := NewAdapter(server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	events, err := adapter.Stream(
		ctx,
		llm.Model{ID: "test-model", Protocol: llm.ProtocolOpenAICompletions, BaseURL: server.URL + "/v1"},
		llm.Context{Messages: []llm.Message{&llm.UserMessage{
			Content: []llm.UserContent{&llm.TextContent{Text: "Wait."}},
		}}},
		llm.StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	select {
	case event := <-events:
		if event.Type != llm.EventStart {
			t.Fatalf("expected start event, got %q", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for start event")
	}
	cancel()

	select {
	case event := <-events:
		if event.Type != llm.EventError {
			t.Fatalf("expected error event, got %q", event.Type)
		}
		if !errors.Is(event.Err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", event.Err)
		}
		if event.Message == nil || event.Message.StopReason != "aborted" {
			t.Fatalf("unexpected cancellation message: %#v", event.Message)
		}
		if event.Message.ErrorMessage == "" {
			t.Fatal("cancellation message is missing error metadata")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancellation event")
	}
}
