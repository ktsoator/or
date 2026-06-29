package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ktsoator/or/llm"
)

// captureThinkingRequest streams a reasoning request and returns the decoded
// "thinking" object from the request body the adapter sent, so tests can assert
// on the display field that the SDK writes there.
func captureThinkingRequest(t *testing.T, adaptive bool, display llm.ThinkingDisplay) map[string]any {
	t.Helper()
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		anthropicSSE(w, "message_start", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"test-model","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
		anthropicSSE(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		anthropicSSE(w, "content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`)
		anthropicSSE(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		anthropicSSE(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`)
		anthropicSSE(w, "message_stop", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	model := llm.Model{
		ID:        "test-model",
		Protocol:  llm.ProtocolAnthropicMessages,
		Provider:  "test",
		BaseURL:   server.URL,
		Reasoning: true,
		Input:     []llm.ModelInput{llm.Text},
		MaxTokens: 4096,
	}
	if adaptive {
		yes := true
		model.Compatibility = &llm.AnthropicMessagesCompatibility{ForceAdaptiveThinking: &yes}
	}
	stream, err := NewAdapter(nil).Stream(context.Background(), model, anthropicTestContext(), llm.StreamOptions{
		APIKey:          "test",
		Reasoning:       llm.ModelThinkingHigh,
		ProtocolOptions: &llm.AnthropicStreamOptions{ThinkingDisplay: display},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for range stream {
	}

	var decoded struct {
		Thinking map[string]any `json:"thinking"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode request body: %v\n%s", err, body)
	}
	if decoded.Thinking == nil {
		t.Fatalf("request body has no thinking object: %s", body)
	}
	return decoded.Thinking
}

// Omitted display reaches the wire for adaptive models, alongside thinking:adaptive.
func TestApplyThinkingOmittedAdaptive(t *testing.T) {
	thinking := captureThinkingRequest(t, true, llm.ThinkingDisplayOmitted)
	if thinking["type"] != "adaptive" {
		t.Fatalf("thinking.type = %v, want adaptive", thinking["type"])
	}
	if thinking["display"] != "omitted" {
		t.Fatalf("thinking.display = %v, want omitted", thinking["display"])
	}
}

// Omitted display also reaches budget-based (non-adaptive) thinking; the budget
// still travels.
func TestApplyThinkingOmittedBudget(t *testing.T) {
	thinking := captureThinkingRequest(t, false, llm.ThinkingDisplayOmitted)
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking.type = %v, want enabled", thinking["type"])
	}
	if thinking["display"] != "omitted" {
		t.Fatalf("thinking.display = %v, want omitted", thinking["display"])
	}
	if _, ok := thinking["budget_tokens"]; !ok {
		t.Fatalf("budget thinking lost budget_tokens: %#v", thinking)
	}
}

// An unset display defaults to summarized, matching prior behavior and the API
// default, on both thinking forms.
func TestApplyThinkingDefaultsToSummarized(t *testing.T) {
	for _, adaptive := range []bool{true, false} {
		thinking := captureThinkingRequest(t, adaptive, "")
		if thinking["display"] != "summarized" {
			t.Fatalf("adaptive=%t thinking.display = %v, want summarized", adaptive, thinking["display"])
		}
	}
}
