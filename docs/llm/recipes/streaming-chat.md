# Streaming chat

## What this builds

A terminal client prints text as it arrives, keeps the final assembled message, records stream errors, and still drains the channel after timeout or cancellation.

Choose `Stream` when time to first token matters or the UI distinguishes text, thinking, and tool progress. Choose `Complete` when only the terminal message is needed.

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

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	events, err := llm.Stream(ctx, model,
		llm.Prompt("Write a three-line poem about Go concurrency."),
		llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err) // setup failed; no stream was created
	}

	var final *llm.AssistantMessage
	var streamErr error
	for event := range events { // always read until the channel closes
		switch event.Type {
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			final = event.Message
		case llm.EventError:
			final = event.Message
			streamErr = event.Err
		}
	}

	if streamErr != nil {
		if final != nil {
			log.Printf("partial text: %q", final.Text())
		}
		log.Fatal(streamErr)
	}
	if final == nil {
		log.Fatal("stream closed without a terminal message")
	}
	fmt.Printf("\nstop=%s tokens=%d\n", final.StopReason, final.Usage.TotalTokens)
}
```

Run with `DEEPSEEK_API_KEY` set:

```sh
go run .
```

## Event lifecycle

```text
EventStart
  → EventTextStart
  → EventTextDelta ...
  → EventTextEnd
  → EventDone | EventError
  → channel close
```

Reasoning and tool calls use their own start/delta/end events. Non-terminal events include `Partial`, a snapshot of the response assembled so far. Terminal events use `Message` instead.

## Why the receive loop is structured this way

`Stream` returns an unbuffered channel. Cancelling `ctx` asks the adapter and HTTP request to stop, but it does not replace channel consumption. Returning from the loop immediately can leave the producer blocked while it publishes its terminal event. Record the error, continue ranging until close, then return it.

`Stream` itself can return an immediate error for invalid options, a missing adapter, or request construction. Provider and decoding failures after startup arrive as `EventError`.

## Integration guidance

| Concern | Recommended handling |
|---|---|
| HTTP streaming | Flush `EventTextDelta`; do not expose `Partial` wholesale on every event |
| Client disconnect | Cancel context, then keep an internal goroutine draining the LLM channel |
| Tool execution | Wait for `EventDone`; streamed arguments may be incomplete |
| Retry | SDK retries occur before or during setup; do not replay a partially displayed answer without a product policy |
| Backpressure | Slow consumers directly slow the producer because the channel is unbuffered |

There is no stream `Close` or `Abort` method. Cancellation is controlled by context, and completion is observed by channel close.
