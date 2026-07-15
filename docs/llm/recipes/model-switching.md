# Changing models in a conversation

The example uses DeepSeek for a draft, then sends the same history to MiniMax for review. The two models use different request and response protocols.

The application stores model-service-neutral `Message` values. It does not manually convert history when the next turn selects another model or protocol.

## Use cases

| Scenario | Switching approach |
|---|---|
| One model drafts and another reviews | Store the complete first response, then add an explicit review request |
| Fallback when the default model is unavailable | Keep the original `Context` and retry with a fallback model |
| Cost or latency tiers | Select a model per turn using application policy |
| Image model extracts information, then text model continues | Convert for target capability and preserve required information as text |
| Switch during a tool loop | Keep complete tool calls and results, and supply tool definitions again |

Model selection is not stored in `Context`. Every `Complete` or `Stream` call must receive the target `Model` explicitly.

## Before running the example

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=...
export MINIMAX_CN_API_KEY=...
```

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	draft := llm.GetModel("deepseek", "deepseek-v4-flash")
	review := llm.GetModel("minimax-cn", "MiniMax-M2.7")

	conversation := llm.PromptWithSystem(
		"Produce concise, operationally safe answers.",
		"Draft a database migration checklist.",
	)
	first := complete(ctx, draft, conversation)
	fmt.Printf("[%s] %s\n", draft.Provider, first.Text())

	conversation.Messages = append(conversation.Messages, &first)
	conversation.Messages = append(conversation.Messages,
		llm.UserText("Review the checklist for missing rollback steps."))
	second := complete(ctx, review, conversation)
	fmt.Printf("[%s] %s\n", review.Provider, second.Text())
}

func complete(ctx context.Context, model llm.Model,
	input llm.Context) llm.AssistantMessage {
	requestCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	response, err := llm.Complete(requestCtx, model, input,
		llm.StreamOptions{MaxTokens: 800})
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

The program sets a two-minute overall deadline and a 45-second deadline for each model call. The second turn reuses the same `Context`, preserving the system prompt and complete message history.

## Checks before switching

| Check | API or field |
|---|---|
| Target model exists | `LookupModel` |
| Target protocol is registered | `SupportsProtocol` or `GetRunnableModels` |
| Target provider credentials are configured | `AuthStatus` |
| Text, image, and reasoning capability | `Model.Input`, `Model.Reasoning` |
| Context window and output limit | `Model.ContextWindow`, `Model.MaxTokens` |

These checks establish local routability only. They do not prove that credentials are valid or that the live model accepts a request; run an integration request against every production target.

## History transformation

Protocol adapters invoke `TransformMessages` automatically before serialization.
Pass the original `Context.Messages`; do not rewrite or replace history first.
The canonical rules for image degradation, reasoning signatures, failed
messages, and tool results are in
[Messages and context](../conversations.md#history-and-model-transformation).

Transformation affects only the copy sent with the current request, so the
original images, signatures, and tool calls remain available when switching
back. Call `TransformMessages` explicitly only to test the transformed history
or implement a custom protocol adapter.

## System prompts, tools, and results

- `SystemPrompt` and `Tools` belong to `Context`, not `Messages`. Pass the complete `Context` when switching, or explicitly recreate these fields.
- Tool definitions are not recovered from old assistant messages. Supply them again in `Context.Tools` when the target model may continue calling tools.
- Every assistant tool call needs a matching result. Missing results are replaced with synthetic errors to keep protocol history valid.
- Historical `Usage`, `ResponseID`, and model identity continue to describe the original response; they are not rewritten for the target model.
- Append the new answer as another `AssistantMessage`; do not overwrite the original model response.

## Capability and semantic differences

Convertible messages do not imply identical behavior. The target may have a smaller context window, lack image or reasoning support, use different tool-selection behavior, or interpret system instructions differently.

Make the handoff request explicit, such as “review the previous answer” or “compress without changing the conclusion.” Do not assume the target knows why it was selected, and do not treat hidden reasoning from the previous model as shared context.

## Boundaries

- Both protocol packages must be imported and both credentials configured.
- Switching providers does not migrate provider-side caches or server-side conversation state.
- The application still owns context-window management. Check `IsContextOverflow` and compact old history before retrying.
- Store the original assistant message, including signatures and tool calls; saving only `Text()` loses replay metadata.
- Model output semantics can differ even when message transport is compatible. Evaluate the handoff prompt for the selected pair.
- On switch failure, preserve original history and current model selection; do not commit an incomplete target-model response to the canonical conversation.
- Models can use different prices and token accounting. Record each `AssistantMessage.Usage` separately and do not reprice old responses with the target model.
- Fallback requests can repeat text or tool calls. Use idempotency keys for side effects and define which turns may be retried.

See [Saving and restoring conversations](conversation-persistence.md) for JSON storage of the complete typed history.
