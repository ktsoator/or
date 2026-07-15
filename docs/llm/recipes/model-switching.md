# Model switching

## What this builds

One conversation uses DeepSeek through OpenAI Chat Completions for drafting, then sends the same history to MiniMax through Anthropic Messages for review.

The application stores provider-neutral `Message` values. It does not manually convert the history when the next turn chooses another model or protocol.

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	ctx := context.Background()
	draft := llm.GetModel("deepseek", "deepseek-v4-flash")
	review := llm.GetModel("minimax-cn", "MiniMax-M2.7")

	history := []llm.Message{
		llm.UserText("Draft a database migration checklist."),
	}
	first := complete(ctx, draft, history)
	fmt.Printf("[%s] %s\n", draft.Provider, first.Text())

	history = append(history, &first)
	history = append(history,
		llm.UserText("Review the checklist for missing rollback steps."))
	second := complete(ctx, review, history)
	fmt.Printf("[%s] %s\n", review.Provider, second.Text())
}

func complete(ctx context.Context, model llm.Model,
	history []llm.Message) llm.AssistantMessage {
	response, err := llm.Complete(ctx, model,
		llm.NewContext(history...), llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		log.Fatal(err)
	}
	return response
}
```

Configure both credentials:

```sh
export DEEPSEEK_API_KEY=...
export MINIMAX_CN_API_KEY=...
go run .
```

## Transformation behavior

Each adapter calls `TransformMessages` before serialization. It creates a target-specific copy and leaves the stored history unchanged.

| Stored content | Target-specific behavior |
|---|---|
| Image sent to text-only model | Replace with a text placeholder |
| Reasoning from the same model | Preserve compatible thinking and signatures |
| Reasoning from another model | Drop provider-private reasoning content |
| Tool-call IDs | Normalize for the target provider and update matching results |
| Failed or aborted assistant turn | Remove from replay |
| Tool call without a result | Insert a synthetic error result |

## Design constraints

- Both protocol packages must be imported and both credentials configured.
- Switching providers does not migrate provider-side caches or server-side conversation state.
- The application still owns context-window management. Check `IsContextOverflow` and compact old history before retrying.
- Store the original assistant message, including signatures and tool calls; saving only `Text()` loses replay metadata.
- Model output semantics can differ even when message transport is compatible. Evaluate the handoff prompt for the selected pair.

See [Conversation persistence](conversation-persistence.md) for JSON storage of the complete typed history.
