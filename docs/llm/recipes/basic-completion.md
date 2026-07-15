# Basic completion

## Purpose

Send one text prompt and receive the complete assistant message.

## Prerequisites

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

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
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		log.Fatal("model is not runnable")
	}

	response, err := llm.Complete(
		context.Background(), model,
		llm.Prompt("Explain a goroutine in one sentence."),
		llm.StreamOptions{MaxTokens: 256},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
	fmt.Printf("stop=%s tokens=%d cost=$%.6f\n",
		response.StopReason,
		response.Usage.TotalTokens,
		response.Usage.Cost.Total)
}
```

## Behavior

- `LookupModel` does not panic for an unknown ID.
- `SupportsProtocol` confirms that this process imported the adapter.
- `Complete` consumes a streaming response internally.
- A non-nil error can accompany a partial message.
