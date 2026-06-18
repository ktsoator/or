package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
			Model    string `json:"model"`
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
		if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Errorf("unexpected messages: %#v", request.Messages)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"think \"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"carefully\",\"content\":\"hello \"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"world\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	registry := llm.NewRegistry()
	if err := registry.Register(NewProvider(server.Client())); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	events, err := llm.NewClient(registry).Stream(
		context.Background(),
		llm.Model{
			ID:       "test-model",
			API:      API,
			Provider: "openai",
			BaseURL:  server.URL + "/v1",
		},
		llm.Context{
			SystemPrompt: "You are helpful.",
			Messages: []llm.Message{{
				Role:    llm.RoleUser,
				Content: []llm.Content{{Type: llm.ContentText, Text: "Say hello."}},
			}},
		},
		llm.StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var deltas string
	var thinkingDeltas string
	var message *llm.AssistantMessage
	for event := range events {
		switch event.Type {
		case llm.EventTextDelta:
			deltas += event.Delta
		case llm.EventThinkingDelta:
			thinkingDeltas += event.Delta
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
	if message == nil {
		t.Fatal("stream did not emit a final message")
	}
	if message.StopReason != "stop" {
		t.Fatalf("unexpected stop reason: %q", message.StopReason)
	}
	if len(message.Content) != 2 || message.Content[0].Thinking != "think carefully" || message.Content[1].Text != "hello world" {
		t.Fatalf("unexpected response content: %#v", message.Content)
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

	provider := NewProvider(server.Client())
	events, err := provider.Stream(
		context.Background(),
		llm.Model{ID: "test-model", API: API, Provider: "openai", BaseURL: server.URL + "/v1"},
		llm.Context{
			Tools: []llm.ToolDefinition{{
				Name:        "get_weather",
				Description: "Get the weather for a city",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
			}},
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: []llm.Content{{Type: llm.ContentText, Text: "Weather in Paris?"}}},
				{Role: llm.RoleAssistant, Content: []llm.Content{
					{Type: llm.ContentThinking, Thinking: "I should call get_weather."},
					{Type: llm.ContentToolCall, ToolCall: &llm.ToolCall{
						ID: "call_1", Name: "get_weather", Arguments: `{"city":"Paris"}`,
					}},
				}},
				{Role: llm.RoleToolResult, ToolCallID: "call_1", ToolName: "get_weather", Content: []llm.Content{
					{Type: llm.ContentText, Text: "Sunny, 20C"},
				}},
			},
		},
		llm.StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var ended *llm.ToolCall
	var message *llm.AssistantMessage
	for event := range events {
		switch event.Type {
		case llm.EventToolCallEnd:
			ended = event.ToolCall
		case llm.EventDone:
			message = event.Message
		case llm.EventError:
			t.Fatalf("stream error: %v", event.Err)
		}
	}

	if ended == nil {
		t.Fatal("stream did not emit a tool call end event")
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
	if len(message.Content) != 1 || message.Content[0].Type != llm.ContentToolCall || message.Content[0].ToolCall == nil {
		t.Fatalf("unexpected response content: %#v", message.Content)
	}
	if call := message.Content[0].ToolCall; call.ID != "call_2" || call.Arguments != `{"city":"Paris"}` {
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

	provider := NewProvider(server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	events, err := provider.Stream(
		ctx,
		llm.Model{ID: "test-model", API: API, BaseURL: server.URL + "/v1"},
		llm.Context{Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.Content{{Type: llm.ContentText, Text: "Wait."}},
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
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancellation event")
	}
}
