# Streaming chat

## Purpose

Render text and reasoning as they arrive, then read the final message from the terminal event.

## Program

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
	events, err := llm.Stream(context.Background(), model,
		llm.Prompt("Write a three-line poem."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for event := range events {
		switch event.Type {
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			fmt.Printf("\n%d tokens\n", event.Message.Usage.TotalTokens)
		case llm.EventError:
			log.Printf("stream failed: %v", event.Err)
		}
	}
}
```

## Lifecycle constraints

- Continue receiving until the event channel closes.
- Drain the channel even when business logic no longer needs deltas.
- Do not execute tool calls from an `EventError` message.
- Use `context.WithCancel` for cancellation and keep receiving the terminal event.
