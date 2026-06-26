# Reasoning and thinking

`StreamOptions.Reasoning` is a provider-neutral effort level. Each adapter maps
it to the target provider's native form—Anthropic adaptive or budget thinking,
or OpenAI-compatible reasoning fields—and clamps it to the levels supported by
the selected model. Non-reasoning models ignore it, so the same option is safe
to set on any model.

```go
options := llm.StreamOptions{Reasoning: llm.ModelThinkingHigh}
response, err := llm.Complete(ctx, model, llm.Prompt("..."), options)
```

## Effort levels

A higher level lets the model spend more tokens thinking before it answers,
trading latency and cost for quality on hard problems. Leaving `Reasoning`
empty uses the model's own default.

| Level | Effect |
|---|---|
| `ModelThinkingOff` | Disable thinking entirely |
| `ModelThinkingMinimal` | Smallest thinking budget |
| `ModelThinkingLow` | Light reasoning |
| `ModelThinkingMedium` | Balanced reasoning |
| `ModelThinkingHigh` | Extended reasoning for hard tasks |
| `ModelThinkingXHigh` | Maximum thinking budget |

## Check what a model supports

Not every model accepts every level. `SupportedThinkingLevels` reports the
levels a model accepts, and `ClampThinkingLevel` snaps a requested level to the
nearest supported one. `Stream` and `Complete` clamp automatically, but calling
it yourself is useful to drive a UI or to skip the option when a model cannot
reason.

```go
levels := llm.SupportedThinkingLevels(model)
if len(levels) == 0 {
	// Model has no reasoning support; do not offer the control.
}

// Snap a user's choice to something the model accepts.
requested := llm.ModelThinkingXHigh
effective := llm.ClampThinkingLevel(model, requested)
if effective != requested {
	log.Printf("model caps thinking at %s", effective)
}

response, err := llm.Complete(ctx, model, input, llm.StreamOptions{
	Reasoning: effective,
})
```

`Model.Reasoning` is a quick boolean check for whether a model reasons at all.

## Read the thinking back

While streaming, reasoning arrives in its own block—`EventThinkingStart`,
`EventThinkingDelta`, `EventThinkingEnd`—before the answer text, so you can
render it separately from the final reply.

```go
for event := range events {
	switch event.Type {
	case llm.EventThinkingDelta:
		fmt.Fprint(thinkingPane, event.Delta)
	case llm.EventTextDelta:
		fmt.Fprint(answerPane, event.Delta)
	}
}
```

From a completed message, the reasoning is a `ThinkingContent` block in
`response.Content`. `Thinking` holds the text; `ThinkingSignature` carries the
provider signature replayed on later turns; `Redacted` marks thinking the
provider withheld.

```go
for _, block := range response.Content {
	if t, ok := block.(*llm.ThinkingContent); ok && !t.Redacted {
		fmt.Println("reasoning:", t.Thinking)
	}
}
```

## Anthropic thinking display

On the Anthropic protocol, `ThinkingDisplay` controls how reasoning is returned
without changing whether the model reasons. An empty value defaults to
summarized thinking.

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplaySummarized,
	},
}
```

`ThinkingDisplayOmitted` withholds the thinking text while retaining the
signature needed for multi-turn tool use. Use it when the application must not
display reasoning content but still needs valid history for follow-up requests.

```go
options := llm.StreamOptions{
	Reasoning: llm.ModelThinkingHigh,
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ThinkingDisplay: llm.ThinkingDisplayOmitted,
	},
}
```

With `ThinkingDisplayOmitted`, no `EventThinkingDelta` events arrive and the
`ThinkingContent` block is marked `Redacted`.

## Conversation continuity

Reasoning metadata needed by a provider—such as Anthropic signatures and
OpenRouter encrypted reasoning—is retained in assistant messages and replayed
when required by later tool calls. When the target model changes, the library
preserves, downgrades, or omits reasoning content according to compatibility.
See [Conversations](conversations.md) for model switching and persistence.
