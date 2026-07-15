# Testing

You can test code built on `llm` without calling a real provider. Two techniques
cover most needs: build result values by hand to test the code that reads
responses, and point a model at a local mock server to test the full request
path.

## Test response handling with plain values

`AssistantMessage`, `Context`, and the content blocks are ordinary structs. Code
that consumes a response — rendering, extracting tool calls, branching on stop
reason — takes an `AssistantMessage`, so factor that code into a function and
call it with a value you construct in the test. No network, no keys.

```go
// Code under test: turn a response into display text.
func renderReply(msg llm.AssistantMessage) string {
	if msg.StopReason == llm.StopReasonError {
		return "error: " + msg.ErrorMessage
	}
	return msg.Text()
}

func TestRenderReply(t *testing.T) {
	msg := llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content:    []llm.AssistantContent{&llm.TextContent{Text: "hello"}},
	}
	if got := renderReply(msg); got != "hello" {
		t.Fatalf("got %q", got)
	}
}
```

Build any shape you need: add a `ToolCall` to `Content` to test tool handling,
set `StopReason` to `StopReasonLength` to test truncation, or populate `Usage` to
test cost accounting. The same holds for input — assemble a `Context` with
`UserText`, `AssistantText`, and `ToolResult` to exercise history logic.

## Test the request path against a mock server

To test the code that actually calls `Stream` or `Complete`, stand up an
`httptest` server that speaks the provider's wire format, then point a `Model` at
it with `BaseURL`. Register the matching provider package as usual, and pass any
non-empty `APIKey` so key resolution is satisfied.

An OpenAI-compatible endpoint streams Server-Sent Events — one `data:` line per
chunk, terminated by `data: [DONE]`:

```go
import (
	"net/http"
	"net/http/httptest"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func TestComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
		}))
	defer server.Close()

	model := llm.Model{
		ID:       "test-model",
		Provider: "test",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL:  server.URL + "/v1",
	}

	msg, err := llm.Complete(context.Background(), model,
		llm.Prompt("hello"), llm.StreamOptions{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text() != "hi" {
		t.Fatalf("got %q", msg.Text())
	}
}
```

The same server works for `Stream`: range over the events and assert on the
`EventTextDelta` sequence and the terminal `EventDone`.

Add a cancellation test for every consumer that can return early. Cancel the
context while the mock server is streaming, continue receiving until the
channel closes, and assert that the consumer goroutine exits. This catches code
that abandons the unbuffered event channel and leaves the producer blocked.

Use an explicit `Client` when a test changes adapters or provider overrides.
This avoids mutating the package defaults shared by parallel tests; see
[Clients and registries](clients-and-registries.md).

## Test a tool loop

Vary the mock server's response by request so it returns a tool call first and a
final answer second. Counting requests is enough to drive one full loop:

```go
var calls int
server := httptest.NewServer(http.HandlerFunc(
	func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		calls++
		if calls == 1 {
			// Turn 1: ask for a tool call.
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\",\"days\":3}"}}]},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			// Turn 2: answer with the tool result in context.
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"Pack light."},"finish_reason":null}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		}
		io.WriteString(w, "data: [DONE]\n\n")
	}))
defer server.Close()
```

Run your loop against this server and assert that it decoded the call, appended a
`ToolResult`, and reached the final text on the second turn. This exercises the
loop logic — the [tool-loop checklist](tools.md#run-the-tool-loop) items — end to
end without a provider.

For an Anthropic-compatible target, set `Protocol: llm.ProtocolAnthropicMessages`
and emit that protocol's event shapes instead.
