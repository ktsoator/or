# Testing strategy

Code built on `llm` can be tested at three layers. Only the adapter request path needs a simulated provider protocol; business result handling and most tool policy can use ordinary Go values.

| Layer | Subject | Network | Recommended form |
|---|---|---|---|
| Result handling | Text rendering, stop reasons, usage, diagnostics | None | Construct `AssistantMessage` values |
| Application flow | History, tool loops, retry policy | None or local | Plain values or a stateful mock |
| Adapter path | Request serialization, SSE, events, error mapping | Local HTTP | `httptest.Server` and an explicit `Model` |

## Test response handling with plain values

`AssistantMessage`, `Context`, and content blocks are ordinary structs. Factor response handling into a function that accepts `AssistantMessage`, then cover error, truncation, tool-call, and usage branches directly.

```go
func renderReply(msg llm.AssistantMessage) string {
	if msg.StopReason == llm.StopReasonError {
		return "error: " + msg.ErrorMessage
	}
	return msg.Text()
}

func TestRenderReply(t *testing.T) {
	msg := llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content: []llm.AssistantContent{
			&llm.TextContent{Text: "hello"},
		},
	}
	if got := renderReply(msg); got != "hello" {
		t.Fatalf("got %q", got)
	}
}
```

Vary the values to cover other branches:

- add a `ToolCall` to `Content`;
- use `StopReasonLength` or `StopReasonAborted`;
- populate `Usage` and `Diagnostics`;
- construct input history with `UserText`, `AssistantText`, and `ToolResult`.

## Test the request path against a mock server

The complete OpenAI-compatible SSE test is maintained only in the [mock-provider task guide](recipes/mock-testing.md). The test path consists of:

1. Return target-protocol events from `httptest.NewServer`.
2. Construct a `Model` whose `BaseURL` points at the server.
3. Register the matching adapter or create an explicit `Client`.
4. Pass a non-empty test key to satisfy local credential validation.
5. Assert the request body, delta order, terminal event, and final message.

A mock test does not prove that a live provider is verified. It proves only that the adapter handles the simulated wire behavior.

## Test a tool loop

A tool-loop test should make the mock server vary its response by request count: return a tool call first, then return final text after the next request includes the `ToolResult`. Assert that:

- `DecodeToolCall` handles the name and arguments;
- every call receives a result with the matching ID;
- failed tools set `IsError`;
- the loop stops on a final stop reason or its iteration limit;
- application retries cannot repeat side-effecting tools.

See [Tool definitions and calls](tools.md) for the type and message contract,
and [Executing tool calls](recipes/tool-loop.md) for the complete application loop.

## Cancellation, concurrency, and isolation

- Add a cancellation test for every stream consumer that can return early. Continue receiving after cancellation until the channel closes, and assert that the goroutine exits.
- Tests that mutate adapters or provider overrides should use an explicit `Client` instead of shared package defaults.
- Reuse one test `http.Client`; do not create a Transport for every assertion.
- Close the `httptest.Server` after the test. An application-owned Transport may call `CloseIdleConnections`.

See [Clients and registries](clients-and-registries.md) for construction and ownership.

## Run

Compile and run the LLM package, examples, and related tests:

```sh
go test ./llm/... ./example/llm/...
```

Existing tests in that command use local data or mock servers and do not require a live provider account.
