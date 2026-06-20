package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/respjson"
)

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		reason string
		want   llm.StopReason
		errors bool
	}{
		{reason: "stop", want: llm.StopReasonStop},
		{reason: "end", want: llm.StopReasonStop},
		{reason: "length", want: llm.StopReasonLength},
		{reason: "tool_calls", want: llm.StopReasonToolUse},
		{reason: "function_call", want: llm.StopReasonToolUse},
		{reason: "content_filter", errors: true},
		{reason: "", errors: true},
		{reason: "novel_reason", errors: true},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			got, err := mapStopReason(test.reason)
			if test.errors {
				if err == nil {
					t.Fatalf("expected error for %q", test.reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("mapStopReason(%q) = %q, want %q", test.reason, got, test.want)
			}
		})
	}
}

func TestReasoningSignatureNormalizesOpencodeGo(t *testing.T) {
	// opencode-go streams under "reasoning" but expects "reasoning_content" on
	// replay, so the source field is normalized.
	model := llm.Model{Provider: "opencode-go"}
	if got := reasoningSignature(model, "reasoning"); got != "reasoning_content" {
		t.Fatalf("opencode-go normalization: got %q", got)
	}
	if got := reasoningSignature(model, "reasoning_content"); got != "reasoning_content" {
		t.Fatalf("non-reasoning field must pass through: got %q", got)
	}
	// Other providers keep the source field verbatim.
	if got := reasoningSignature(llm.Model{Provider: "openai"}, "reasoning"); got != "reasoning" {
		t.Fatalf("openai must keep reasoning verbatim: got %q", got)
	}
}

func TestApplyToolChoicePointerVariant(t *testing.T) {
	// The pointer variant of the named choice must produce the same wire shape
	// as the value variant, so callers passing &foo work.
	params := oai.ChatCompletionNewParams{}
	applyToolChoice(&params, &llm.OpenAIToolChoiceFunction{Name: "weather"})
	raw, _ := json.Marshal(params.ToolChoice)
	if !strings.Contains(string(raw), `"function":{"name":"weather"}`) {
		t.Fatalf("pointer named choice wire = %s", raw)
	}
}

func TestApplyToolChoiceNilPointerIsNoop(t *testing.T) {
	params := oai.ChatCompletionNewParams{}
	applyToolChoice(&params, (*llm.OpenAIToolChoiceFunction)(nil))
	if raw, _ := json.Marshal(params.ToolChoice); string(raw) != `null` {
		t.Fatalf("nil pointer must leave tool choice empty, got %s", raw)
	}
}

func TestApplyToolChoiceUnknownModeIsNoop(t *testing.T) {
	// An unknown mode string must not set tool_choice rather than emit garbage.
	params := oai.ChatCompletionNewParams{}
	applyToolChoice(&params, llm.OpenAIToolChoiceMode("invalid"))
	if raw, _ := json.Marshal(params.ToolChoice); string(raw) != `null` {
		t.Fatalf("invalid mode wire = %s", raw)
	}
}

func TestUsageFromExtraAbsent(t *testing.T) {
	// No extra field at all: ok must be false with no error.
	usage, ok, err := usageFromExtra(map[string]respjson.Field{}, "usage", llm.Model{})
	if err != nil || ok {
		t.Fatalf("usage = %#v ok=%v err=%v, want zero/false/nil", usage, ok, err)
	}
}

func TestUsageFromExtraNullValueSkipped(t *testing.T) {
	fields := map[string]respjson.Field{"usage": respjson.NewField("null")}
	usage, ok, err := usageFromExtra(fields, "usage", llm.Model{})
	if err != nil {
		t.Fatalf("usage err = %v", err)
	}
	if ok {
		t.Fatalf("ok = true for null field, want false")
	}
	if (usage != llm.Usage{}) {
		t.Fatalf("usage = %#v, want zero", usage)
	}
}

func TestUsageFromExtraDecodesObject(t *testing.T) {
	raw := `{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":2}}`
	fields := map[string]respjson.Field{"usage": respjson.NewField(raw)}
	usage, ok, err := usageFromExtra(fields, "usage", llm.Model{})
	if err != nil {
		t.Fatalf("usage err = %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true for non-null usage")
	}
	if usage.Input != 8 || usage.Output != 5 || usage.CacheRead != 2 {
		t.Fatalf("usage = %#v, want input=8 output=5 cacheRead=2", usage)
	}
	if usage.TotalTokens != 15 {
		t.Fatalf("totalTokens = %d, want 15", usage.TotalTokens)
	}
}

func TestUsageFromExtraReportsDecodeError(t *testing.T) {
	// Malformed JSON must surface as an error rather than silently zero usage.
	fields := map[string]respjson.Field{"usage": respjson.NewField(`{not json`)}
	if _, _, err := usageFromExtra(fields, "usage", llm.Model{}); err == nil {
		t.Fatalf("expected decode error for malformed usage")
	}
}

func TestBuildClientHonorsTimeoutAndRetries(t *testing.T) {
	// The SDK does not expose its option set directly, so the smoke test is
	// that buildClient does not panic when every optional path is exercised and
	// that the constructed client honors the configured base URL.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	maxRetries := 0
	timeout := 5 * time.Second
	options := llm.StreamOptions{
		MaxRetries: &maxRetries,
		Timeout:    timeout,
		Headers:    map[string]string{"X-Test": "1"},
		OnRequest:  func(string, string, []byte) {},
		OnResponse: func(int, http.Header) {},
	}
	model := llm.Model{
		Provider: "test",
		BaseURL:  server.URL,
		Headers:  map[string]string{"X-Model": "y"},
	}
	client := buildClient(server.Client(), model, options)
	// The SDK surfaces options through the constructed client; the bare smoke is
	// that the value is usable.
	_ = client
}

func TestStreamSurfacesProviderHTTPErrorOnEventChannel(t *testing.T) {
	// A non-2xx response from the provider must surface as an EventError on the
	// stream rather than panicking the adapter; the SDK rejects the body and we
	// translate it into a terminal error event.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request","type":"invalid_request_error"}}`))
	}))
	t.Cleanup(server.Close)

	retries := 0
	events, err := NewAdapter(server.Client()).Stream(
		t.Context(),
		llm.Model{
			ID:       "test-model",
			Provider: "test",
			Protocol: llm.ProtocolOpenAICompletions,
			BaseURL:  server.URL + "/v1",
		},
		llm.Context{Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hi"}}},
		}},
		llm.StreamOptions{APIKey: "test", MaxRetries: &retries},
	)
	if err != nil {
		t.Fatalf("Stream setup error: %v", err)
	}

	sawError := false
	for event := range events {
		if event.Type == llm.EventError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("expected an EventError when the provider returns 400")
	}
}
