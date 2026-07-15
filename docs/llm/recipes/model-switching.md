# Model switching

## Purpose

Use different models with one conversation history, such as one model drafting and another reviewing.

## Core code

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)

draft := llm.GetModel("deepseek", "deepseek-v4-flash")
review := llm.GetModel("anthropic", "claude-sonnet-4-6")

messages := []llm.Message{
	llm.UserText("Draft a migration checklist."),
}

first, err := llm.Complete(ctx, draft,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}

messages = append(messages, &first)
messages = append(messages, llm.UserText("Review the checklist for missing rollback steps."))

second, err := llm.Complete(ctx, review,
	llm.Context{Messages: messages}, llm.StreamOptions{})
```

Both provider credentials must be configured.

## Transformation behavior

- reasoning and signatures are not sent across models;
- images are downgraded for text-only targets;
- tool-call IDs are normalized for the target provider;
- unanswered tool calls receive synthetic error results;
- the original `messages` slice is not permanently rewritten.
