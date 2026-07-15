# Mock-provider testing

## What this builds

An `httptest` server emits OpenAI-compatible SSE, and a test verifies the full `Complete` request path without a provider account or network access.

Save this as `completion_test.go` in a package that depends on `llm`.

## Complete test

```go
package myapp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func TestCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
		}))
	defer server.Close()

	model := llm.Model{
		ID: "test-model", Provider: "test",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL: server.URL + "/v1", MaxTokens: 100,
	}
	message, err := llm.Complete(context.Background(), model,
		llm.Prompt("hello"), llm.StreamOptions{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := message.Text(); got != "hi" {
		t.Fatalf("text = %q, want hi", got)
	}
	if message.StopReason != llm.StopReasonStop {
		t.Fatalf("stop = %q", message.StopReason)
	}
}
```

Run with `go test ./...`. The non-empty request API key satisfies local validation but is never sent outside the test server.

## What to test next

- Inspect the request JSON to assert tools, system prompt, reasoning, and headers.
- Emit `tool_calls` first and a final answer on the second request to test a complete tool loop.
- Emit error status and malformed SSE to verify partial messages and error policy.
- Test `Stream` cancellation and continue receiving until channel close to detect goroutine leaks.
- Use an explicit client and registry when tests register custom adapters or provider overrides, avoiding package-global cross-test state.

Mock tests validate protocol translation, not live-provider compatibility. Keep a small credentialed integration suite for providers used in production.
