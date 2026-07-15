# Error handling

## Error layers

Errors occur at three different stages and require different handling:

| Stage | Signal | Examples |
|---|---|---|
| Local setup | `Stream`/`Complete` returns an error before a stream exists | Invalid options, missing adapter, request construction |
| Runtime stream | `EventError`, or `Complete` returns partial message plus error | Authentication, rate limit, HTTP failure, decoding failure |
| Normal generation stop | Nil error with non-stop `StopReason` | Token limit or tool request |

## Complete program with a reusable policy

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	message, err := complete(context.Background(), model,
		llm.Prompt("Explain context cancellation briefly."))
	if err != nil {
		log.Printf("partial text: %q", message.Text())
		log.Fatal(err)
	}
	fmt.Println(message.Text())
}

func complete(ctx context.Context, model llm.Model,
	input llm.Context) (llm.AssistantMessage, error) {
	message, err := llm.Complete(ctx, model, input,
		llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		// message may contain partial text and usage; caller decides retention.
		return message, fmt.Errorf("complete %s/%s: %w",
			model.Provider, model.ID, err)
	}

	switch message.StopReason {
	case llm.StopReasonStop, llm.StopReasonToolUse:
		return message, nil
	case llm.StopReasonLength:
		return message, fmt.Errorf("output truncated at max token limit")
	case llm.StopReasonAborted:
		return message, context.Canceled
	default:
		return message, fmt.Errorf("generation stopped: %s: %s",
			message.StopReason, message.ErrorMessage)
	}
}
```

## Retry decisions

- Retry transient transport, rate-limit, or provider-availability failures only when the operation is safe to replay.
- Do not retry missing adapters, invalid options, invalid tools, or unknown model IDs without changing configuration.
- SDK retries are controlled by `MaxRetries`; application retries wrap the whole logical request and can duplicate displayed text or tool effects.
- Do not execute tool calls from `EventError`, `StopReasonError`, or `StopReasonAborted` messages.
- For `StopReasonLength`, either accept truncation, raise the cap, or append the partial assistant turn and explicitly ask to continue.

## Context overflow

```go
if llm.IsContextOverflow(message, model.ContextWindow) {
	// Compact, summarize, or remove old messages in application code, then retry.
}
```

`llm` detects explicit provider errors and some silent usage-based overflows. It does not choose which messages to remove.

## Troubleshooting data

Record provider/model ID, protocol, stop reason, response ID, attempt count, latency, and redacted diagnostics. Do not record API keys, full headers, raw request bodies, images, or complete histories by default. See [Troubleshooting](../troubleshooting.md) for symptom-specific checks.
