package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
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
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{"model":"x"}`))
	if _, err := mw(req, option.MiddlewareNext(next)); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}

	if gotMethod != http.MethodPost || gotURL != "https://api.anthropic.com/v1/messages" {
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
	mw := rewriteRequestMiddleware(func(method, url string, body []byte) []byte {
		return []byte(`{"model":"rewritten"}`)
	})

	var forwarded []byte
	var forwardedLen int64
	next := func(req *http.Request) (*http.Response, error) {
		forwarded, _ = io.ReadAll(req.Body)
		forwardedLen = req.ContentLength
		return &http.Response{StatusCode: http.StatusOK}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{"model":"x"}`))
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
	mw := rewriteRequestMiddleware(func(method, url string, body []byte) []byte {
		return nil // leave the body unchanged
	})

	var forwarded []byte
	next := func(req *http.Request) (*http.Response, error) {
		forwarded, _ = io.ReadAll(req.Body)
		return &http.Response{StatusCode: http.StatusOK}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{"model":"x"}`))
	if _, err := mw(req, option.MiddlewareNext(next)); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}

	if string(forwarded) != `{"model":"x"}` {
		t.Fatalf("downstream body = %q, want unchanged", forwarded)
	}
}
