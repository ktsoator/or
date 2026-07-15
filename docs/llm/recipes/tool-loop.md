# Tool loop

## What this builds

A bounded loop lets the model request a typed weather tool, validates and decodes its arguments, executes application code, appends one result per call, and sends the updated history back for a final answer.

`llm` defines and transports tool calls. It never runs them. Authorization, timeouts, idempotency, auditing, and side-effect policy belong to the application.

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

type WeatherArgs struct {
	City string `json:"city" jsonschema:"description=City name,minLength=1"`
	Days int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func main() {
	ctx := context.Background()
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

		for _, call := range response.ToolCalls() {
			args, err := llm.DecodeToolCall[WeatherArgs](weather, call)
			if err != nil {
				result := llm.ToolResult(call.ID, call.Name, err.Error())
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
```

## Loop contract

1. Define `ToolDefinition`; `MustTool` is appropriate for static startup declarations and panics on an invalid definition.
2. Send the tool with the current messages.
3. Append the assistant message before any tool result.
4. Continue only for `StopReasonToolUse`.
5. Match every returned `ToolCall` with exactly one `ToolResultMessage`.
6. Send the expanded history again until normal stop or the application limit.

`DecodeToolCall` validates against the generated schema and decodes into `WeatherArgs`. It can coerce common mistakes such as a numeric string, but coercion is not authorization.

## Failure and security policy

- Inspect `response.Diagnostics` before executing side effects. `partial` or `invalid` recovered arguments should normally be rejected.
- Return decode or business errors to the model with `IsError=true`; do not crash the whole loop for a correctable call.
- Apply per-tool deadlines and cancellation. The example tool is synchronous and deterministic only to keep the LLM flow visible.
- Add allowlists and user authorization independently of the schema. A valid argument can still request a forbidden action.
- Use idempotency keys for writes. Provider retries and application retries must not duplicate external effects.
- Bound turns, concurrent calls, payload size, and total spend.

Protocol-specific forced tool selection is documented in [Tools](../tools.md#protocol-specific-tool-choice).
