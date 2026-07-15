# One-shot text generation

`Complete` waits for a model request to finish and returns a complete `AssistantMessage`. Use it for batch work, service methods that return only final results, and one turn in a conversation or tool loop.

It still reads the provider response through its streaming interface, but the application does not handle intermediate events. Use [Streaming responses](streaming-chat.md) when text, reasoning, or tool-call progress must be shown while generation is in progress.

## When to use Complete

| Scenario | Is `Complete` appropriate? |
|---|---|
| Background generation, summarization, or classification | Yes; only the final result is needed |
| A service method returns after generation finishes | Yes; handle one `AssistantMessage` |
| One turn in a multi-turn conversation | Yes; the application still owns history |
| One model request in a tool loop | Yes; inspect the result before running tools |
| Rendering generated text incrementally | No; use `Stream` |
| Displaying reasoning or tool arguments live | No; consume streaming events |

## Constructing request content

The third argument to `Complete` is a `Context`. Plain-text calls can use these helpers:

| Constructor | Content created |
|---|---|
| `llm.Prompt(text)` | One user message containing text |
| `llm.PromptWithSystem(system, user)` | A system prompt and one user message |
| `llm.NewContext(messages...)` | A context from existing typed messages |
| `llm.Context{...}` | System prompt, messages, and tool definitions together |

For a user-only prompt:

```text
input := llm.Prompt("Summarize Go context cancellation in three sentences.")
```

The system prompt belongs to `Context.SystemPrompt`; it is not appended as an ordinary history message. Conversations, images, and tools require a full `Context`. See [Saving and restoring conversations](conversation-persistence.md), [Sending images](vision.md), and [Executing tool calls](tool-loop.md).

## Run the example

The example looks up a model in the built-in model catalog, verifies its adapter is registered, sends a system prompt and user prompt, then prints text, stop reason, token usage, and estimated cost.

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

func ptr[T any](value T) *T { return &value }

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

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
	response, err := llm.Complete(ctx, model, input,
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

The answer text is provider-generated. A normal response usually ends with `stop=stop`. Token counts come from provider usage metadata; cost is estimated from built-in model catalog prices.

## Request flow

1. `LookupModel` reads `(provider, model ID)` from the built-in model catalog and returns `(Model, bool)`.
2. The side-effect import of `llm/openai` registers the OpenAI Chat Completions adapter during initialization.
3. `Complete` validates `StreamOptions` and obtains an API key from provider configuration or environment variables.
4. The adapter transforms messages, serializes the request, and reads the provider response stream.
5. On `EventDone`, `Complete` returns the final message. On `EventError`, it may return a partial message and an error.

## Request settings used by the example

The program uses `context.WithTimeout` to limit the whole call to 45 seconds and
sets `Temperature` and `MaxTokens` explicitly. `Temperature` is a pointer so an
unset value differs from an explicit number; `MaxTokens` caps output.

`StreamOptions.Timeout` limits one HTTP attempt and does not replace the call
context. See [Request options](../configuration.md) for retries, credentials,
headers, reasoning levels, hooks, and every other field.

## Reading the result

The example reads `Text()`, `StopReason`, and `Usage`. Service code should retain
the complete `AssistantMessage` so content blocks, response IDs, diagnostics,
and partial results remain available. See
[Responses and usage](../results.md) for the canonical fields and stop reasons.

## Failure handling and boundaries

- `GetModel` panics for unknown dynamic input; use `LookupModel` for configuration or user-supplied IDs.
- A catalog model may use an unimplemented protocol. Check `SupportsProtocol` or select from `GetRunnableModels`.
- A missing key fails before the provider call. Use `AuthStatus` when exposing configuration diagnostics.
- A non-nil error can include partial text and usage. Decide whether partial output may be displayed or persisted.
- `Usage.Cost` is an estimate, not a billing record.

## Using Complete in a service

- When model IDs come from configuration or user input, validate them with `LookupModel` at startup or at the request boundary.
- Set a context deadline for every call; `StreamOptions.Timeout` limits only one HTTP attempt.
- Branch on `StopReason` to return text, execute tools, report truncation, or surface failure.
- When `err` is non-nil, decide whether to retain the partial message before returning an application error.
- Logs can include provider, model, response ID, stop reason, usage, and latency. Do not log API keys or complete prompts.

See [Handling request failures](error-handling.md) for a shared response policy, and [Finding models and checking credentials](provider-discovery.md) for dynamic model selection and credential checks.
