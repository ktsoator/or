# Reasoning output

## Purpose

Select a neutral reasoning level and render thinking separately from final answer text.

## Core code

```go
requested := llm.ModelThinkingHigh
effective := llm.ClampThinkingLevel(model, requested)

events, err := llm.Stream(ctx, model,
	llm.Prompt("Solve 37 * 48 and check the result."),
	llm.StreamOptions{Reasoning: effective})
if err != nil {
	log.Fatal(err)
}

for event := range events {
	switch event.Type {
	case llm.EventThinkingDelta:
		fmt.Fprint(os.Stderr, event.Delta)
	case llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventError:
		log.Print(event.Err)
	}
}
```

Anthropic can omit returned thinking text while preserving signatures required by later turns:

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

`ThinkingDisplayOmitted` does not disable reasoning. Thinking tokens still count as output usage.
