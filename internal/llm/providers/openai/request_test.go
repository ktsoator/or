package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

func TestOnRequestMiddlewareObservesBodyAndRestoresIt(t *testing.T) {
	var gotMethod, gotURL string
	var gotBody []byte
	mw := onRequestMiddleware(func(method, url string, body []byte) {
		gotMethod, gotURL, gotBody = method, url, body
	})

	var forwarded []byte
	next := func(req *http.Request) (*http.Response, error) {
		// The downstream request must still see the body the middleware read.
		forwarded, _ = io.ReadAll(req.Body)
		return &http.Response{StatusCode: http.StatusOK}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "https://api.test/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	if _, err := mw(req, option.MiddlewareNext(next)); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}

	if gotMethod != http.MethodPost || gotURL != "https://api.test/v1/chat/completions" {
		t.Fatalf("observed method/url = %q %q", gotMethod, gotURL)
	}
	if string(gotBody) != `{"model":"x"}` {
		t.Fatalf("observed body = %q", gotBody)
	}
	if string(forwarded) != `{"model":"x"}` {
		t.Fatalf("downstream body = %q, want body restored", forwarded)
	}
}

func TestOnResponseMiddlewareObservesEachAttempt(t *testing.T) {
	type seen struct {
		status  int
		headers http.Header
	}
	var calls []seen
	mw := onResponseMiddleware(func(status int, headers http.Header) {
		calls = append(calls, seen{status, headers})
	})

	// Simulate the SDK re-running the middleware chain across a retry: first a
	// 429, then a 200. The hook must fire once per attempt.
	attempts := []*http.Response{
		{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"1"}}},
		{StatusCode: http.StatusOK, Header: http.Header{}},
	}
	for _, resp := range attempts {
		next := func(*http.Request) (*http.Response, error) { return resp, nil }
		if _, err := mw(&http.Request{}, option.MiddlewareNext(next)); err != nil {
			t.Fatalf("middleware returned error: %v", err)
		}
	}

	if len(calls) != 2 {
		t.Fatalf("expected hook to fire twice, got %d", len(calls))
	}
	if calls[0].status != http.StatusTooManyRequests || calls[0].headers.Get("Retry-After") != "1" {
		t.Fatalf("first attempt not observed correctly: %+v", calls[0])
	}
	if calls[1].status != http.StatusOK {
		t.Fatalf("second attempt status = %d, want 200", calls[1].status)
	}
}

func TestOnResponseMiddlewareSkipsNilResponse(t *testing.T) {
	called := false
	mw := onResponseMiddleware(func(int, http.Header) { called = true })
	next := func(*http.Request) (*http.Response, error) { return nil, http.ErrServerClosed }
	if _, err := mw(&http.Request{}, option.MiddlewareNext(next)); err == nil {
		t.Fatal("expected error to propagate")
	}
	if called {
		t.Fatal("hook must not fire when there is no response")
	}
}

func TestMergeExtraFieldsPreservesExisting(t *testing.T) {
	params := oai.ChatCompletionNewParams{}
	params.SetExtraFields(map[string]any{"keep": 1, "shared": "old"})

	mergeExtraFields(&params, map[string]any{"shared": "new", "added": true})

	got := params.ExtraFields()
	if got["keep"] != 1 {
		t.Errorf("keep field lost: %#v", got)
	}
	if got["shared"] != "new" {
		t.Errorf("shared field not overridden: %#v", got["shared"])
	}
	if got["added"] != true {
		t.Errorf("added field missing: %#v", got)
	}
}

func TestMergeExtraFieldsAcceptsNilStartingMap(t *testing.T) {
	// SetExtraFields was never called, so the SDK starts with no map at all.
	params := oai.ChatCompletionNewParams{}
	mergeExtraFields(&params, map[string]any{"a": 1})

	if got := params.ExtraFields()["a"]; got != 1 {
		t.Fatalf("a = %#v, want 1", got)
	}
}

func TestMergedHeaders(t *testing.T) {
	model := llm.Model{Headers: map[string]string{"X-A": "model-a", "X-Both": "model"}}
	opts := llm.StreamOptions{Headers: map[string]string{"X-B": "opt-b", "X-Both": "opts"}}
	got := mergedHeaders(model, opts)

	if got["X-A"] != "model-a" {
		t.Errorf("model header lost: %v", got)
	}
	if got["X-B"] != "opt-b" {
		t.Errorf("options header lost: %v", got)
	}
	if got["X-Both"] != "opts" {
		t.Errorf("options must override model: %v", got)
	}
}

func TestMergedHeadersReturnsNilWhenEmpty(t *testing.T) {
	if got := mergedHeaders(llm.Model{}, llm.StreamOptions{}); got != nil {
		t.Fatalf("expected nil for empty inputs, got %v", got)
	}
}

func TestMergedHeadersOnlyModelHeaders(t *testing.T) {
	model := llm.Model{Headers: map[string]string{"X-A": "model"}}
	got := mergedHeaders(model, llm.StreamOptions{})
	if len(got) != 1 || got["X-A"] != "model" {
		t.Fatalf("merged = %v", got)
	}
}

func TestBuildParamsMaxTokensRoutesByCompat(t *testing.T) {
	model := llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions}
	opts := llm.StreamOptions{MaxTokens: 128}

	std := buildParams(model, nil, nil, opts, resolvedCompat{maxTokensField: "max_completion_tokens"})
	rawStd, _ := json.Marshal(std)
	wireStd := string(rawStd)
	if !strings.Contains(wireStd, `"max_completion_tokens":128`) {
		t.Errorf("wanted max_completion_tokens=128 in %s", wireStd)
	}
	if strings.Contains(wireStd, `"max_tokens":128`) {
		t.Errorf("must not emit max_tokens for standard compat: %s", wireStd)
	}

	legacy := buildParams(model, nil, nil, opts, resolvedCompat{maxTokensField: "max_tokens"})
	rawLegacy, _ := json.Marshal(legacy)
	wireLegacy := string(rawLegacy)
	if !strings.Contains(wireLegacy, `"max_tokens":128`) {
		t.Errorf("wanted max_tokens=128 in %s", wireLegacy)
	}
}

func TestBuildParamsSetsStoreForOpenAIDefault(t *testing.T) {
	// supportsStore is the default for native OpenAI: an explicit store=false
	// keeps the conversation transient.
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{supportsStore: true},
	)
	raw, _ := json.Marshal(params)
	if !strings.Contains(string(raw), `"store":false`) {
		t.Fatalf("wanted store=false in %s", raw)
	}
}

func TestBuildParamsOmitsStoreForNonStandard(t *testing.T) {
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{supportsStore: false},
	)
	raw, _ := json.Marshal(params)
	if strings.Contains(string(raw), `"store"`) {
		t.Fatalf("store must be omitted for non-standard: %s", raw)
	}
}

func TestBuildParamsTemperatureAndProtocolOptions(t *testing.T) {
	temp := 0.42
	opts := llm.StreamOptions{
		Temperature:     &temp,
		ProtocolOptions: &llm.OpenAICompletionsStreamOptions{ToolChoice: llm.OpenAIToolChoiceRequired},
	}
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, opts,
		resolvedCompat{},
	)
	raw, _ := json.Marshal(params)
	wire := string(raw)
	if !strings.Contains(wire, `"temperature":0.42`) {
		t.Errorf("wanted temperature: %s", wire)
	}
	if !strings.Contains(wire, `"tool_choice":"required"`) {
		t.Errorf("wanted tool_choice=required: %s", wire)
	}
}

func TestBuildParamsAddsZAIToolStreamWhenToolsAndCompatSet(t *testing.T) {
	tools := []oai.ChatCompletionToolUnionParam{oai.ChatCompletionFunctionTool(
		shared.FunctionDefinitionParam{Name: "noop"},
	)}
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, tools, llm.StreamOptions{},
		resolvedCompat{zaiToolStream: true},
	)
	if got := params.ExtraFields()["tool_stream"]; got != true {
		t.Fatalf("tool_stream = %#v, want true", got)
	}
}

func TestBuildParamsSkipsZAIToolStreamWithoutTools(t *testing.T) {
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{zaiToolStream: true},
	)
	if _, present := params.ExtraFields()["tool_stream"]; present {
		t.Fatalf("tool_stream must not be set without tools")
	}
}

func TestBuildParamsAlwaysIncludesUsageInStream(t *testing.T) {
	params := buildParams(
		llm.Model{ID: "x", Protocol: llm.ProtocolOpenAICompletions},
		nil, nil, llm.StreamOptions{},
		resolvedCompat{},
	)
	raw, _ := json.Marshal(params)
	if !strings.Contains(string(raw), `"include_usage":true`) {
		t.Fatalf("stream_options.include_usage must be true: %s", raw)
	}
}
