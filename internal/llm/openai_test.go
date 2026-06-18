package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	registry := NewRegistry()
	if err := registry.Register(NewOpenAIProvider(server.Client())); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	events, err := NewClient(registry).Stream(
		context.Background(),
		Model{
			ID:       "test-model",
			API:      OpenAICompletionsAPI,
			Provider: "openai",
			BaseURL:  server.URL + "/v1",
		},
		Context{
			SystemPrompt: "You are helpful.",
			Messages: []Message{{
				Role:    RoleUser,
				Content: []Content{{Type: ContentText, Text: "Say hello."}},
			}},
		},
		StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var deltas string
	var thinkingDeltas string
	var message *AssistantMessage
	for event := range events {
		switch event.Type {
		case EventTextDelta:
			deltas += event.Delta
		case EventThinkingDelta:
			thinkingDeltas += event.Delta
		case EventDone:
			message = event.Message
		case EventError:
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

	provider := NewOpenAIProvider(server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	events, err := provider.Stream(
		ctx,
		Model{ID: "test-model", API: OpenAICompletionsAPI, BaseURL: server.URL + "/v1"},
		Context{Messages: []Message{{
			Role:    RoleUser,
			Content: []Content{{Type: ContentText, Text: "Wait."}},
		}}},
		StreamOptions{APIKey: "test-key"},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	select {
	case event := <-events:
		if event.Type != EventStart {
			t.Fatalf("expected start event, got %q", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for start event")
	}
	cancel()

	select {
	case event := <-events:
		if event.Type != EventError {
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
