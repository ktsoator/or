package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
	"github.com/openai/openai-go/v3/option"
)

func TestOnRequestMiddlewareObservesBodyAndRestoresIt(t *testing.T) {
	var gotMethod, gotURL string
	var gotBody []byte
	mw := onRequestMiddleware(func(method, url string, body []byte) {
		gotMethod, gotURL, gotBody = method, url, body
	})

	var forwarded []byte
	next := func(req *http.Request) (*http.Response, error) {
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

func TestRewriteRequestMiddlewareReplacesBody(t *testing.T) {
	mw := rewriteRequestMiddleware(func(string, string, []byte) []byte {
		return []byte(`{"model":"rewritten"}`)
	})

	var forwarded []byte
	var forwardedLen int64
	next := func(req *http.Request) (*http.Response, error) {
		forwarded, _ = io.ReadAll(req.Body)
		forwardedLen = req.ContentLength
		return &http.Response{StatusCode: http.StatusOK}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "https://api.test/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	if _, err := mw(req, option.MiddlewareNext(next)); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}

	if string(forwarded) != `{"model":"rewritten"}` {
		t.Fatalf("downstream body = %q, want rewritten", forwarded)
	}
	if forwardedLen != int64(len(`{"model":"rewritten"}`)) {
		t.Fatalf("ContentLength = %d, want %d", forwardedLen, len(`{"model":"rewritten"}`))
	}
}

func TestRewriteRequestMiddlewareNilKeepsBody(t *testing.T) {
	mw := rewriteRequestMiddleware(func(string, string, []byte) []byte {
		return nil
	})

	var forwarded []byte
	next := func(req *http.Request) (*http.Response, error) {
		forwarded, _ = io.ReadAll(req.Body)
		return &http.Response{StatusCode: http.StatusOK}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "https://api.test/v1/chat/completions", strings.NewReader(`{"model":"x"}`))
	if _, err := mw(req, option.MiddlewareNext(next)); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}

	if string(forwarded) != `{"model":"x"}` {
		t.Fatalf("downstream body = %q, want unchanged", forwarded)
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

func TestMergedHeaders(t *testing.T) {
	model := llm.Model{Headers: map[string]string{"X-A": "model-a", "X-Both": "model"}}
	options := llm.StreamOptions{Headers: map[string]string{"X-B": "opt-b", "x-both": "opts"}}
	got := mergedHeaders(model, options)

	if got["X-A"] != "model-a" {
		t.Errorf("model header lost: %v", got)
	}
	if got["X-B"] != "opt-b" {
		t.Errorf("options header lost: %v", got)
	}
	if got["X-Both"] != "opts" {
		t.Errorf("options must override model: %v", got)
	}
	if len(got) != 3 {
		t.Errorf("case variants must collapse to one header: %v", got)
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
