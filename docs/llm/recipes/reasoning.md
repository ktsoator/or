# Reasoning output

## What this builds

A streaming command requests the closest supported reasoning level, renders thinking separately from answer text, and reads the final usage record.

Reasoning is a request hint, not a guarantee that visible thinking text will be returned. Providers use different native controls, and a model can perform reasoning while omitting its text.

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	requested := llm.ModelThinkingHigh
	effective := llm.ClampThinkingLevel(model, requested)
	fmt.Fprintf(os.Stderr, "supported=%v requested=%s effective=%s\n",
		llm.SupportedThinkingLevels(model), requested, effective)

	events, err := llm.Stream(context.Background(), model,
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
		log.Fatal(streamErr)
	}
	if final != nil {
		fmt.Printf("\noutput tokens=%d\n", final.Usage.Output)
	}
}
```

## How neutral levels are mapped

`ModelThinkingLevel` provides `off`, `minimal`, `low`, `medium`, `high`, and `xhigh`. `SupportedThinkingLevels` derives the levels advertised by the model. `ClampThinkingLevel` selects the nearest available level; non-reasoning models effectively ignore the request.

The adapter translates the effective level into protocol-native fields. Do not place provider-specific reasoning JSON in `RewriteRequest` unless the typed mapping cannot represent a required endpoint feature.

## Anthropic display control

Anthropic protocol options can request omitted visible thinking while preserving reasoning behavior:

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

`ThinkingDisplayOmitted` does not turn reasoning off. Thinking tokens can still be counted as output, and signatures needed to continue the same model conversation remain part of the assistant message.

## Boundaries

- Thinking text and signatures may contain sensitive information. Do not log them by default.
- When switching models, provider-specific reasoning blocks are removed rather than replayed to another model.
- Output token limits include reasoning consumption on providers that account for it that way.
- Catalog reasoning metadata can be stale. Verify behavior against the selected endpoint.
