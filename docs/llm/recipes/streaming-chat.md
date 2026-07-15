# Streaming responses

`Stream` returns an event channel. Applications can handle text, reasoning, and tool-call deltas while generation is in progress, then obtain the complete `AssistantMessage` at the end.

Use `Stream` when an interface must show output immediately or when content must be handled by event type. Use `Complete` from [One-shot text generation](basic-completion.md) when only the final message is needed.

## When to use Stream

| Scenario | Handling |
|---|---|
| Render text live in a terminal, web page, or desktop interface | Handle `EventTextDelta` |
| Display reasoning separately from the final answer | Handle thinking and text events separately |
| Show tool-call preparation progress | Read tool-call events, but wait for termination before execution |
| Measure time to first content | Record the time of the first delta event |
| Only the final message matters | Use `Complete`; no event loop is needed |

`Stream` does not write events to SSE, WebSocket, or a terminal. The application chooses the output protocol and owns disconnect handling, flush frequency, and partial-content visibility.

## Before running the example

The example uses a DeepSeek model. Install the dependency and set an API key:

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## Complete program

The program prints every text delta when it arrives. It continues to consume events until the channel closes, including after an error or context cancellation.

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
		log.Fatal(err) // no event channel was returned
	}

	var final *llm.AssistantMessage
	var streamErr error
	for event := range events { // keep reading until the channel closes
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

Run it:

```sh
go run .
```

Text is printed as `EventTextDelta` arrives. On normal completion, the program prints the stop reason and total token count.

## Event order

Built-in protocol adapters emit `EventStart`, followed by content events, then exactly one `EventDone` or `EventError`, and finally close the channel. One text block normally follows this order:

```text
EventStart
  → EventTextStart
  → EventTextDelta ...
  → EventTextEnd
  → EventDone | EventError
  → channel close
```

Reasoning and tool calls each have their own start, delta, and end events. `Partial` on a non-terminal event is a snapshot of the response assembled so far. `EventDone` and `EventError` use `Message` for the completed or partial message.

A response can contain multiple content blocks. `ContentIndex` identifies the corresponding entry in `AssistantMessage.Content`. Blocks of different types can arrive in model-service order, so applications must not assume that a response contains only one text block.

## Events handled by the example

The program above handles three event types: `EventTextDelta` writes newly
arrived text, `EventDone` stores the final message, and `EventError` stores the
partial message and error. A reasoning view or tool-call progress indicator adds
the corresponding branches without changing channel consumption or terminal
handling.

See [Streaming events](../streaming.md#event-reference) for the canonical event
list, valid fields, and the semantics of `ContentIndex` and `Partial`. This guide
does not maintain a second field table.

## Consume events until close

`Stream` returns an unbuffered channel. Cancelling `ctx` requests that the model service stop, but does not replace channel consumption. Returning from the loop immediately can block the sender while it publishes a terminal event.

When an error arrives, save `event.Err` and continue reading until the channel closes before handling it. `Stream` returns an error directly when options are invalid, an adapter is not registered, or a request cannot be created. Model-service and decoding failures after the request starts arrive as `EventError`.

Handle the two error paths separately:

| Failure point | `Stream` result | Event channel |
|---|---|---|
| Before the request starts | `events == nil`, `err != nil` | Not created |
| After the request starts | Initial `err == nil` | Emits `EventError`, then closes |

Continue consuming after context cancellation. The terminal message uses `StopReasonAborted`, and `Err` contains the context cancellation or deadline error.

## Integration handling

| Situation | Handling |
|---|---|
| Continuously writing to a client | Write and flush after `EventTextDelta`; do not resend the entire `Partial` for every event |
| Client disconnect | Cancel the context, then keep consuming events until the channel closes |
| Tool calls | Wait for `EventDone` before execution; streamed arguments may be incomplete |
| Retry | Whether to repeat a request after partial output was shown is an application interaction-policy decision |
| Slow consumption | The channel is unbuffered; slower event handling directly slows response reading |
| Multiple content blocks | Use `ContentIndex` to update the matching UI region; do not append every delta to one string by default |

Streams have no `Close` or `Abort` method. Cancel the request through context and use channel closure to confirm processing has ended.

## Production checklist

- Set a context deadline for the whole call and cancel it when the client disconnects.
- Send only new content from delta events instead of retransmitting the full `Partial`.
- Move slow logging, database writes, and telemetry out of the event-consumption loop.
- After `EventDone`, inspect `Message.StopReason`; tool use and output truncation still require application handling.
- After `EventError`, decide whether displayed partial content should be retained. Avoid automatic replay that duplicates output.
- Do not log raw reasoning, tool arguments, images, or complete messages without explicit redaction and access control.
