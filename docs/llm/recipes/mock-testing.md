# Testing with a local mock server

`httptest.Server` can simulate a model service locally. Without a real account or external network access, a test can cover request serialization, streaming response parsing, and final message assembly.

The test below simulates an OpenAI Chat Completions SSE response. Save it as `completion_test.go` in a Go package that uses `llm`.

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

Run it:

```sh
go test ./...
```

The example API key satisfies request validation and is sent only to the local test server.

## Additional paths to cover

- Inspect request JSON to assert tools, system prompts, reasoning options, and headers.
- Emit `tool_calls` first and a final answer on the second request to test a complete tool loop.
- Emit error status and malformed SSE to verify partial messages and error policy.
- Test `Stream` cancellation and continue receiving until channel close to detect goroutine leaks.
- Use a custom client and independent registries when testing custom adapters or provider overrides, avoiding package-global cross-test state.

## Test scope

Local mock tests validate application logic and protocol translation. They do not validate authentication, rate limits, field differences, or live behavior of a real model service. Keep a small credentialed integration suite for model services used in production.
