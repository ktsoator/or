# Conversation persistence

## What this builds

The application completes one turn, stores the complete typed `Context` as JSON, restores it, appends another user message, and continues the conversation.

`llm` is stateless. The history slice passed on a request is the conversation; the package does not create sessions or write a database.

## Complete program

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	ctx := context.Background()
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	history := []llm.Message{llm.UserText("Name one Go web framework.")}

	first, err := llm.Complete(ctx, model,
		llm.NewContext(history...), llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err)
	}
	history = append(history, &first)

	stored, err := json.MarshalIndent(llm.Context{Messages: history}, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("conversation.json", stored, 0o600); err != nil {
		log.Fatal(err)
	}

	raw, err := os.ReadFile("conversation.json")
	if err != nil {
		log.Fatal(err)
	}
	var restored llm.Context
	if err := json.Unmarshal(raw, &restored); err != nil {
		log.Fatal(err)
	}
	restored.Messages = append(restored.Messages,
		llm.UserText("Which year was it first released?"))

	second, err := llm.Complete(ctx, model, restored,
		llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(second.Text())
}
```

## What must be stored

Store complete message JSON, not only rendered text. Assistant content can include reasoning signatures, tool calls, provider/model identity, usage, diagnostics, and stop reason. These fields affect later replay and troubleshooting.

For row-oriented storage, use `MarshalMessage` and `UnmarshalMessage` on one message at a time. Unknown roles, unknown content types, and malformed JSON return errors.

## Production policy

- Treat serialized history as sensitive. It may contain prompts, tool results, images, and provider signatures.
- Encrypt or access-control storage, use restrictive file/database permissions, and avoid whole-history logs.
- Define retention, deletion, and tenant ownership outside `llm`.
- Before each request, enforce `Model.ContextWindow`; `llm` detects overflow but does not summarize history.
- Append `&AssistantMessage`, not a value converted to text, so the concrete message type and metadata survive.
- A `SystemPrompt` belongs to the request `Context`; persist it separately if the application needs it across turns.
