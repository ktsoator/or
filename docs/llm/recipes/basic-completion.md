# Basic completion

## What this builds

A command-line program resolves a catalog model, verifies that its adapter is registered, sends a system prompt and user prompt, then prints text, stop reason, usage, and estimated cost.

Use `Complete` for batch work, HTTP handlers that return only a finished response, and one turn inside an application-owned conversation or tool loop. It still consumes the provider's streaming API internally.

## Prerequisites

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

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func ptr[T any](value T) *T { return &value }

func main() {
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok {
		log.Fatal("model is not present in the embedded catalog")
	}
	if !llm.SupportsProtocol(model.Protocol) {
		log.Fatalf("protocol %q has no registered adapter", model.Protocol)
	}

	input := llm.PromptWithSystem(
		"You are a concise Go reviewer.",
		"When should a Go service use a channel instead of a mutex?",
	)
	response, err := llm.Complete(context.Background(), model, input,
		llm.StreamOptions{
			Temperature: ptr(0.2),
			MaxTokens:   400,
		})
	if err != nil {
		log.Printf("partial response: %q", response.Text())
		log.Fatal(err)
	}

	fmt.Println(response.Text())
	fmt.Printf("\nstop=%s input=%d output=%d cost=$%.6f\n",
		response.StopReason,
		response.Usage.Input,
		response.Usage.Output,
		response.Usage.Cost.Total,
	)
}
```

Run it:

```sh
go run .
```

The answer text is provider-generated. A normal response ends with `stop=stop`; token counts and cost depend on provider usage metadata.

## Call flow

1. `LookupModel` reads the embedded catalog and returns `(Model, bool)` without panicking.
2. The side-effect import registers the OpenAI Chat Completions adapter.
3. `Complete` validates options and resolves `DEEPSEEK_API_KEY`.
4. The adapter transforms history, serializes the provider request, and reads its stream.
5. `Complete` returns the message from `EventDone`, or a partial message with an error from `EventError`.

## Parameters and results

| Value | Meaning |
|---|---|
| `Temperature` | Pointer distinguishes “not specified” from an explicit value; provider acceptance varies |
| `MaxTokens` | Output cap; `0` leaves the protocol-specific default behavior |
| `response.Text()` | All text content blocks concatenated in order |
| `response.StopReason` | Why generation ended; branch on this before tool execution |
| `response.Usage` | Provider-reported tokens plus catalog-priced cost estimate |

## Failure and boundary conditions

- `GetModel` panics for unknown dynamic input; use `LookupModel` for configuration or user-supplied IDs.
- A catalog model may use an unimplemented protocol. Check `SupportsProtocol` or select from `GetRunnableModels`.
- A missing key fails before the provider call. Use `AuthStatus` when exposing configuration diagnostics.
- A non-nil error can include partial text and usage. Decide whether partial output may be displayed or persisted.
- `Usage.Cost` is not a billing record.

See [Error handling](error-handling.md) for a reusable response policy and [Model and auth discovery](provider-discovery.md) for dynamic model selection.
