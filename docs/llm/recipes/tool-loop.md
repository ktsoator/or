# Executing tool calls

After a model requests a tool, the application validates and decodes the arguments, runs its own tool implementation, appends the result to history, and asks the model to continue. The example uses a weather tool and limits execution to eight turns.

`llm` defines and transports tool calls; it does not run them. Authorization, timeouts, idempotency, auditing, and side-effect policy belong to the application.

## Responsibility boundary

| Stage | What `llm` provides | Application responsibility |
|---|---|---|
| Tool declaration | Generate JSON Schema from a Go struct | Design names, descriptions, and argument constraints |
| Tool request | Parse `ToolCall` and preserve call IDs | Dispatch names only to allowed implementations |
| Argument handling | Coerce, validate, and decode against schema | Authorization, business rules, and resource-scope checks |
| Tool execution | No executor is provided in the current material | Timeout, cancellation, concurrency, audit, and side effects |
| Result return | Construct `ToolResultMessage` | Redact output and decide whether errors are retryable |
| Loop control | Stop reasons and typed history | Turn, token, cost, and total-duration limits |

## Before running the example

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

type WeatherArgs struct {
	City string `json:"city" jsonschema:"description=City name,minLength=1"`
	Days int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	weather := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast")
	messages := []llm.Message{
		llm.UserText("What should I pack for three days in Beijing?"),
	}

	for turn := 0; turn < 8; turn++ {
		response, err := llm.Complete(ctx, model, llm.Context{
			Messages: messages,
			Tools:    []llm.ToolDefinition{weather},
		}, llm.StreamOptions{MaxTokens: 800})
		if err != nil {
			log.Fatal(err)
		}

		messages = append(messages, &response) // assistant must precede results
		switch response.StopReason {
		case llm.StopReasonStop:
			fmt.Println(response.Text())
			return
		case llm.StopReasonToolUse:
			// Continue below.
		default:
			log.Fatalf("generation stopped: %s: %s",
				response.StopReason, response.ErrorMessage)
		}

		calls := response.ToolCalls()
		if len(calls) == 0 {
			log.Fatal("model stopped for tool use without a tool call")
		}
		for _, call := range calls {
			if argumentsRecovered(response.Diagnostics, call.ID) {
				result := llm.ToolResult(call.ID, call.Name,
					"tool arguments were incomplete; call the tool again")
				result.IsError = true
				messages = append(messages, result)
				continue
			}
			args, err := llm.DecodeToolCall[WeatherArgs](weather, call)
			if err != nil {
				log.Printf("invalid tool call %s: %v", call.ID, err)
				result := llm.ToolResult(call.ID, call.Name,
					"invalid tool arguments; correct them and try again")
				result.IsError = true
				messages = append(messages, result)
				continue
			}

			// Replace this deterministic stub with an authorized tool implementation.
			text := fmt.Sprintf("%s: sunny, 25C, for %d days", args.City, args.Days)
			messages = append(messages, llm.ToolResult(call.ID, call.Name, text))
		}
	}
	log.Fatal("tool loop exceeded 8 turns")
}

func argumentsRecovered(diagnostics []llm.Diagnostic, callID string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Type != llm.DiagnosticToolArgumentsRecovered {
			continue
		}
		id, _ := diagnostic.Details["toolCallId"].(string)
		if id == callID {
			return true
		}
	}
	return false
}
```

The weather result is a deterministic stub. A real implementation needs its own tool deadline plus user authorization and resource-scope checks before external access.

## Defining tools

The example uses `MustTool` for a fixed startup declaration and
`DecodeToolCall` to validate and decode returned arguments. Use `NewTool`, which
returns an error, for dynamically built definitions. See
[Tool definitions and calls](../tools.md) for all constructors, schema tags, and
generic validation APIs.

Schema validation checks argument shape; it does not authorize access to a
city, file, or account.

## Tool-call order

1. Define `ToolDefinition`; `MustTool` is appropriate for static startup declarations and panics on an invalid definition.
2. Send the tool with the current messages.
3. Append the assistant message before any tool result.
4. Continue only for `StopReasonToolUse`.
5. Match every returned `ToolCall` with exactly one `ToolResultMessage`.
6. Send the expanded history again until normal stop or the application limit.

`DecodeToolCall` validates against the generated schema and decodes into `WeatherArgs`. It can coerce common format differences such as a numeric string, but successful coercion is not authorization.

## Multiple tools and result return

One response can contain multiple `ToolCall` values. Iterate over `response.ToolCalls()` and append one result with the matching call ID for every call:

- On success, use `ToolResult(call.ID, call.Name, text)`.
- For argument or business errors, still return `ToolResultMessage` with `IsError = true` so the model can correct the call.
- Do not expose internal stacks, database errors, or credentials in tool error text; keep details in application logs.
- Independent read-only calls may run concurrently, but concurrency must be bounded and each result must retain the correct call ID.
- Calls with ordering dependencies or side effects must follow application sequencing rules; model-return order is not a safety policy.

Tool results can contain text or images. Construct `ToolResultMessage.Content` manually for images; the `ToolResult` helper creates text-only results.

## Streaming tool calls

With `Stream`, tool arguments arrive incrementally through `EventToolCallDelta`. Delta JSON can be incomplete, and even `EventToolCallEnd` can contain best-effort recovered arguments.

Wait for `EventDone`, then read calls from the final `AssistantMessage.ToolCalls()` and inspect `Diagnostics`. `DiagnosticToolArgumentsRecovered` means arguments were not parsed as complete strict JSON; side-effecting operations should normally reject automatic execution.

## Failure and security boundaries

- Inspect `response.Diagnostics` before executing side effects. `partial` or `invalid` recovered arguments should normally be rejected.
- Return decode or business errors to the model with `IsError=true`; do not crash the whole loop for a correctable call.
- Apply per-tool deadlines and cancellation. The example tool is synchronous and deterministic only to keep the LLM flow visible.
- Add allowlists and user authorization independently of the schema. A valid argument can still request a forbidden action.
- Use idempotency keys for writes. Provider retries and application retries must not duplicate external effects.
- Bound turns, concurrent calls, payload size, and total spend.
- Treat tool names as untrusted input and dispatch only to explicitly registered implementations. A model must not select arbitrary functions or commands by name.
- Tool output becomes conversation history and can be read by later model calls. Remove credentials, internal paths, and unnecessary personal data before returning it.
- After context cancellation, stop starting new tools and propagate cancellation to running tool implementations.

Protocol-specific forced tool selection is documented in [Tools](../tools.md#protocol-specific-tool-choice).
