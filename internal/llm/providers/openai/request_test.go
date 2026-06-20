package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
