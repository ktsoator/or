# Saving and restoring conversations

`llm` does not store conversation state. The application passes a `Context` on every call; to continue a conversation after a process restart, the application must persist and restore that `Context`.

The JSON representation of `Context` includes the system prompt, messages, and tool definitions. Messages retain their concrete types, so the restored context can be sent to a model service again.

## Scope

| Requirement | What `llm` provides | Application responsibility |
|---|---|---|
| Continue on the next turn | `Context.Messages` and typed messages | Store, read, and append messages |
| Restore after a restart | JSON encoding for `Context` | Database or file storage |
| Persist tool calls | JSON for `ToolCall` and `ToolResultMessage` | Tool execution state and idempotency records |
| Identify a conversation | No conversation ID is defined in the current material | Conversation ID, user, and tenant ownership |
| Select the next model | Every call receives `Model` explicitly | Persist model selection or choose again |
| Limit history size | `Model.ContextWindow`, `IsContextOverflow` | Remove, compact, or summarize old messages |

## Before running the example

The example uses a DeepSeek model and writes the conversation to `conversation.json` in the current directory:

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## Complete program

The program completes one request, writes the complete `Context` to a JSON file, reads the file, adds a new user message, and makes another request.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	conversation := llm.PromptWithSystem(
		"Answer briefly.",
		"Name one Go web framework.",
	)

	first := complete(model, conversation)
	conversation.Messages = append(conversation.Messages, &first)

	stored, err := json.MarshalIndent(conversation, "", "  ")
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

	second := complete(model, restored)
	fmt.Println(second.Text())
}

func complete(model llm.Model, input llm.Context) llm.AssistantMessage {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	response, err := llm.Complete(ctx, model, input,
		llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err)
	}
	return response
}
```

Run it:

```sh
go run .
```

The first run creates `conversation.json`. It contains the system prompt, the first user message, and the complete assistant message from the first turn. The second request continues after that history.

## What to store

Store the complete `Context`, not only text displayed to the user. An assistant message can contain reasoning, tool calls, model-service and model identifiers, token usage, diagnostics, and a stop reason. Later calls, replay, and troubleshooting can depend on this information.

For one-message-at-a-time storage, use `MarshalMessage` and `UnmarshalMessage`. They write a role to JSON so a message can be restored as a `UserMessage`, `AssistantMessage`, or `ToolResultMessage`. Unknown roles, unknown content types, and malformed JSON return errors.

`Context` stores `SystemPrompt`, `Messages`, and `Tools`; see
[Messages and context](../conversations.md) for the canonical fields and message
types. It does not contain a conversation ID, creation time, user or tenant ID,
database version, or the `Model` for the next turn. Applications normally wrap
`Context` in their own record type.

## Choosing a storage method

| Method | Appropriate use | Behavior |
|---|---|---|
| `json.Marshal(Context)` | Read and write a complete short conversation | Stores system prompt, messages, and tools together |
| `MarshalMessage` | Row-oriented database storage or JSON Lines | Stores one message with role and content types |
| Application record | Conversation ID, version, tenant, or audit fields are needed | Embeds `Context` or individual messages in an application schema |

When storing messages individually, persist `SystemPrompt` and `Tools` separately. Restore messages in their original turn order.

## Conversation recovery flow

1. Create a `Context` and append user messages.
2. When a request finishes, append `&AssistantMessage` to `Context.Messages`.
3. Serialize and store the complete `Context`.
4. Read the JSON and unmarshal it into `Context`.
5. Append the next user message, then pass the restored `Context` to `Complete` or `Stream`.

`Context` persists `SystemPrompt` and `Tools`. When a storage design saves only `Messages`, it must save those fields separately.

## Turn order and concurrent updates

- Preserve the order of user messages, assistant messages, and tool results. When an assistant requests tools, store the complete assistant message before its matching results.
- `Complete` can return a partial assistant message together with a non-nil `err`. The application must decide explicitly whether partial output is stored.
- When multiple requests update one conversation, use a transaction, version number, or optimistic lock so a late write cannot overwrite newer history.
- Confirm business completion after persistence succeeds, or use a retryable transaction, to avoid a model response that is not recorded.
- `llm` does not lock `Context`. Do not mutate the same message slice from another goroutine while a request uses it.

## Storage boundaries

- History can include prompts, tool results, images, and signatures returned by model services; treat it as sensitive data.
- Use encryption, access control, and least-privilege file or database permissions. Do not log complete conversations.
- Retention periods, deletion rules, and tenant ownership belong to the application's storage system.
- Before every request, constrain history to `Model.ContextWindow`. `llm` can report context overflow but does not compress or summarize history.
- Append `&AssistantMessage`, rather than only its text, to preserve the message type and metadata.
- Keep the original history when changing models; protocol adapters create a target-model copy before sending. See [Changing models in a conversation](model-switching.md).
- The current JSON has no application-level schema version. For long-term storage, define versioning and migrations on the enclosing application record.
