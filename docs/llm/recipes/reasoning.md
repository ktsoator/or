# Requesting reasoning

`StreamOptions.Reasoning` requests a model reasoning level. A streaming response can handle reasoning content separately from final text.

A reasoning level expresses a preference, not a guarantee that visible reasoning text will be returned. A model can reason internally while returning only its final answer.

## Scope

| Requirement | Handling |
|---|---|
| Request more or less reasoning effort | Set `StreamOptions.Reasoning` |
| Separate reasoning from the final answer live | Handle thinking and text events separately |
| Only the final answer is needed | Use `Complete`; `Reasoning` still applies |
| Hide visible Anthropic reasoning | Set `AnthropicStreamOptions.ThinkingDisplay` |
| Read exact reasoning tokens | Unified `Usage` has no separate field; use model-service data |

Reasoning is useful for mathematics, code analysis, complex planning, and multi-step decisions, but higher levels commonly increase latency and token consumption. Simple extraction, formatting, or classification may not benefit.

## Before running the example

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	requested := llm.ModelThinkingHigh
	effective := llm.ClampThinkingLevel(model, requested)
	fmt.Fprintf(os.Stderr, "supported=%v requested=%s effective=%s\n",
		llm.SupportedThinkingLevels(model), requested, effective)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	events, err := llm.Stream(ctx, model,
		llm.Prompt("Solve 37 * 48 and verify the result."),
		llm.StreamOptions{Reasoning: effective, MaxTokens: 1000})
	if err != nil {
		log.Fatal(err)
	}

	var final *llm.AssistantMessage
	var streamErr error
	for event := range events {
		switch event.Type {
		case llm.EventThinkingStart:
			fmt.Fprintln(os.Stderr, "--- thinking ---")
		case llm.EventThinkingDelta:
			fmt.Fprint(os.Stderr, event.Delta)
		case llm.EventTextStart:
			fmt.Println("--- answer ---")
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			final = event.Message
		case llm.EventError:
			final, streamErr = event.Message, event.Err
		}
	}
	if streamErr != nil {
		if final != nil {
			log.Printf("partial answer: %q", final.Text())
		}
		log.Fatal(streamErr)
	}
	if final == nil {
		log.Fatal("stream closed without a terminal message")
	}
	fmt.Printf("\nstop=%s output tokens=%d\n",
		final.StopReason, final.Usage.Output)
}
```

Reasoning deltas go to standard error and final-answer text goes to standard output, allowing callers to display or redirect them separately. The program still consumes events until channel close.

## Selecting a reasoning level

`SupportedThinkingLevels` returns the levels declared by the selected model, and
`ClampThinkingLevel` converts a user choice into a value that model accepts.
Record both the requested and effective values so the interface matches the
actual request. The canonical level list, clamping order, and token semantics
are in [Reasoning options](../reasoning.md#effort-levels).

The protocol adapter translates the effective level into endpoint-specific fields. Use `RewriteRequest` only when typed configuration cannot express a required endpoint feature.

## Reading reasoning events

The example writes new reasoning text on `EventThinkingDelta`, writes the final
answer on `EventTextDelta`, and stores the message from `EventDone` or
`EventError`. A response may contain several reasoning and text blocks; use
`ContentIndex` when each block updates a separate region.

`ThinkingContent` in the final `AssistantMessage.Content` can also contain
signatures or redaction markers, so do not persist only the reasoning text shown
on screen. See [Streaming events](../streaming.md#event-reference) for the full
field contract.

## Controlling visible reasoning

Anthropic Messages can omit visible reasoning while retaining the signatures
needed to continue a conversation. See
[Reasoning § Anthropic thinking display](../reasoning.md#anthropic-thinking-display)
for the option, code, and token semantics; this task guide does not duplicate
protocol configuration.

`ThinkingDisplayOmitted` controls returned content only; it does not change whether the model reasons or how it is billed. Only the Anthropic protocol currently implements this display option. Passing Anthropic-specific options to another protocol fails before the request starts.

## Tokens, results, and history

- `Usage.Output` records output usage reported by the model service. Some services include reasoning consumption, but unified `Usage` has no separate reasoning-token field.
- The relationship between `MaxTokens` and reasoning budget is model-service specific. Heavy reasoning can leave less room for the final answer or end at the output limit.
- When continuing with the same model, persist the complete `AssistantMessage` so reasoning signatures needed for conversation or tool use are retained.
- When changing models, `TransformMessages` removes model-specific reasoning instead of sending it to the new model as ordinary text.

## Boundaries

- Thinking text and signatures may contain sensitive information. Do not log them by default.
- When switching models, model-service-specific reasoning blocks are removed rather than replayed to another model.
- Some model services count reasoning consumption toward output limits.
- Built-in model catalog reasoning metadata can be stale. Verify behavior against the selected model.
- Do not treat visible reasoning as a factual source or audit record; validate final answers and tool arguments.
- Before exposing reasoning to end users, define product policy, access controls, and retention periods.
