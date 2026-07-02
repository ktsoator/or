# Streaming

Use `Stream` to process text and reasoning as they are generated.

**1. Start the stream.** `Stream` returns a channel immediately; the request
runs in the background and delivers events as they arrive.

```go
events, err := llm.Stream(
	context.Background(),
	model,
	llm.Prompt("Explain Go channels briefly."),
	llm.StreamOptions{Reasoning: llm.ModelThinkingHigh},
)
if err != nil {
	log.Fatal(err)
}
```

**2. Consume events with a type switch.** Print text and reasoning deltas as
they come, capture the final message on `EventDone`, and stop on `EventError`.

```go
var finalMessage *llm.AssistantMessage
for event := range events {
	switch event.Type {
	case llm.EventThinkingDelta, llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventDone:
		finalMessage = event.Message
	case llm.EventError:
		log.Fatal(event.Err)
	}
}
```

**3. Read the final message** for the stop reason, token usage, and cost once
the channel closes.

```go
fmt.Printf("\nstop=%s tokens=%d cost=$%.6f\n",
	finalMessage.StopReason,
	finalMessage.Usage.TotalTokens,
	finalMessage.Usage.Cost.Total,
)
```

Thinking events are emitted only when the selected model and provider expose
reasoning content.

<details>
<summary>Full program</summary>

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	events, err := llm.Stream(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels briefly."),
		llm.StreamOptions{Reasoning: llm.ModelThinkingHigh},
	)
	if err != nil {
		log.Fatal(err)
	}

	var finalMessage *llm.AssistantMessage
	for event := range events {
		switch event.Type {
		case llm.EventThinkingDelta:
			fmt.Print(event.Delta)
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			finalMessage = event.Message
		case llm.EventError:
			log.Fatal(event.Err)
		}
	}
	if finalMessage == nil {
		log.Fatal("stream closed without a final message")
	}
	fmt.Printf("\nstop=%s tokens=%d cost=$%.6f\n",
		finalMessage.StopReason,
		finalMessage.Usage.TotalTokens,
		finalMessage.Usage.Cost.Total,
	)
}
```

</details>

## Event reference

A stream opens with `EventStart`, emits one `start → delta… → end` group per
content block (text, thinking, or tool call, possibly interleaved), and closes
with exactly one terminal event:

```mermaid
flowchart LR
    start(["EventStart"]) --> blocks

    subgraph blocks["one group per content block"]
        direction LR
        bs["…Start"] --> bd["…Delta<br/><small>× many</small>"] --> be["…End"]
    end

    blocks --> outcome{"outcome"}
    outcome -->|success| done(["EventDone<br/><small>Message = final AssistantMessage</small>"])
    outcome -->|failure / cancel| err(["EventError<br/><small>Err + partial Message</small>"])

    classDef ok stroke:#16a34a,stroke-width:2px;
    classDef bad stroke:#dc2626,stroke-width:2px;
    class done ok;
    class err bad;
```

Every non-terminal event carries a `Partial` snapshot; the `…` prefix stands for
`Text`, `Thinking`, or `ToolCall`.

| Event | Meaning | Main fields |
|---|---|---|
| `EventStart` | The provider stream started | `Partial` |
| `EventTextStart` | A text block started | `ContentIndex`, `Partial` |
| `EventTextDelta` | A text fragment arrived | `ContentIndex`, `Delta`, `Partial` |
| `EventTextEnd` | A text block completed | `ContentIndex`, `Content`, `Partial` |
| `EventThinkingStart` | A reasoning block started | `ContentIndex`, `Partial` |
| `EventThinkingDelta` | A reasoning fragment arrived | `ContentIndex`, `Delta`, `Partial` |
| `EventThinkingEnd` | A reasoning block completed | `ContentIndex`, `Content`, `Partial` |
| `EventToolCallStart` | A tool call block started | `ContentIndex`, `ToolCall`, `Partial` |
| `EventToolCallDelta` | A raw tool-argument JSON fragment arrived | `ContentIndex`, `Delta`, `ToolCall`, `Partial` |
| `EventToolCallEnd` | A tool call finished streaming, arguments parsed best-effort | `ContentIndex`, `ToolCall`, `Partial` |
| `EventDone` | The request completed successfully | `Message` |
| `EventError` | The request failed or was cancelled | `Err`, `Message` |

`EventDone.Message` is the final assistant message and contains content, usage,
cost, and stop reason. `EventError.Message` may contain partial content and
usage. The channel emits exactly one terminal event and then closes. See
[Reading responses](results.md) for how to interpret the final message: stop
reasons, token usage and cost, diagnostics, and context-overflow detection.

Events from different content blocks may be interleaved. Use `ContentIndex` to
associate deltas with their block. Every non-terminal event carries a `Partial`
snapshot of the assistant message built so far.

## Tool-call deltas and diagnostics

`EventToolCallDelta.Delta` contains raw partial JSON. `EventToolCallEnd` carries
the call with arguments parsed best-effort: malformed or truncated JSON degrades
to the fields received so far, or to an empty object. Validate arguments before
use, collect tool calls while streaming, and execute them only after
`EventDone`. Never execute calls from a response that ends with `EventError`.

When arguments could not be parsed strictly, the response records a
`tool_arguments_recovered` entry in `Message.Diagnostics`. Its recovery `mode`
is `repaired`, `partial`, or `invalid`. Inspect diagnostics before executing a
tool with side effects. A safe application declines `partial` and `invalid`
arguments and returns a tool error so the model can retry.

## Cancellation

Cancelling the request context stops an in-flight request. The stream emits one
`EventError` whose message reports `StopReasonAborted`, then closes.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

events, err := llm.Stream(ctx, model, input, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}

// Call cancel() from elsewhere, for example when the user presses Stop.
for event := range events {
	switch event.Type {
	case llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventError:
		fmt.Printf("\nstopped: %s\n", event.Message.StopReason)
	}
}
```

Use the independent per-attempt `Timeout` option for transport deadlines; see
[Request configuration](configuration.md).
