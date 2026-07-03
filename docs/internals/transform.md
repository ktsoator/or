# Switching models

The canonical conversation history is provider-neutral, but not every model can
accept every block exactly as stored. `TransformMessages` projects the stored
history into a request-safe form for the target model before an adapter converts
it to wire format.

The important detail: transformation happens per request. It does not mutate the
stored history, so switching to a text-only model does not permanently discard
images, and switching away from a reasoning model does not erase its signatures.
`Stream` and `Complete` apply it automatically.

## Transformation order

`TransformMessages` chains three passes, each returning a new slice:

```go
func TransformMessages(messages []Message, model Model, normalizeToolCallID func(string) string) []Message {
	transformed := downgradeUnsupportedImages(messages, model)      // (1)!
	transformed = reconcileAssistantHistory(transformed, model, normalizeToolCallID) // (2)!
	return synthesizeOrphanedToolResults(transformed)               // (3)!
}
```

1.  Replace image blocks with placeholder text when the target model does not
    list `Image` in `Model.Input`. Consecutive images collapse into one
    placeholder.
2.  Rewrite prior assistant turns for the target model: keep reasoning and
    signatures for the same model, downgrade them across models, and normalize
    tool-call IDs when crossing providers.
3.  Repair transcripts where an assistant tool call has no matching tool result.

## Reasoning and signatures

Reasoning is model-specific. A signature or redacted payload is only safe to
replay when provider, protocol, and model ID all match — the `sameModel`
condition. `reconcileThinking` encodes the full decision for one reasoning block:

```go
func reconcileThinking(content *ThinkingContent, sameModel bool) AssistantContent {
	if content == nil {
		return nil
	}
	if content.Redacted { // (1)!
		if sameModel {
			return content
		}
		return nil
	}
	if sameModel && content.ThinkingSignature != "" { // (2)!
		return content
	}
	if strings.TrimSpace(content.Thinking) == "" { // (3)!
		return nil
	}
	if sameModel {
		return content
	}
	return &TextContent{Text: content.Thinking} // (4)!
}
```

1.  Redacted reasoning is an opaque encrypted payload only the original model can
    consume: kept for the same model, dropped otherwise.
2.  A signed block is replayed intact for the same model, even when its text is
    empty.
3.  Empty, unsigned reasoning carries nothing and is dropped.
4.  Reasoning from another model is downgraded to plain text, so its content
    survives without a signature the target would reject.

Text and tool-call signatures follow the same principle: opaque provider metadata
is kept only for the model that produced it.

## Tool-call identifiers

Different protocols accept different tool-call ID shapes. When an assistant turn
crosses models, the adapter supplies a `normalizeToolCallID` function. If the ID
changes, the change is recorded and the matching `ToolResultMessage.ToolCallID`
is remapped later in the same forward pass, so the transcript stays consistent.

The final pass enforces the tool protocol invariant: every assistant tool-call
batch must receive one result per call before another user or assistant turn
begins. Missing results are synthesized as error tool results carrying
`"No result provided"`, and assistant turns that ended in an error or
cancellation are dropped entirely, because they may hold partial reasoning or
half-streamed tool calls.

## Overflow detection

`IsContextOverflow` is separate from transformation. It inspects a completed or
failed `AssistantMessage` and recognizes three shapes of provider context
overflow:

```go
func IsContextOverflow(message AssistantMessage, contextWindow int64) bool {
	// Case 1: error message patterns.
	if message.StopReason == StopReasonError && message.ErrorMessage != "" { // (1)!
		if !matchesAny(nonOverflowPatterns, message.ErrorMessage) &&
			matchesAny(overflowPatterns, message.ErrorMessage) {
			return true
		}
	}

	// Case 2: silent overflow (z.ai style) - successful but usage exceeds context.
	if contextWindow > 0 && message.StopReason == StopReasonStop { // (2)!
		if message.Usage.Input+message.Usage.CacheRead > contextWindow {
			return true
		}
	}

	// Case 3: length-stop overflow (Xiaomi MiMo style) - input truncated to fill
	// the window, leaving no room for output.
	if contextWindow > 0 && message.StopReason == StopReasonLength && message.Usage.Output == 0 { // (3)!
		inputTokens := message.Usage.Input + message.Usage.CacheRead
		if float64(inputTokens) >= float64(contextWindow)*0.99 {
			return true
		}
	}

	return false
}
```

1.  Most providers return an error whose text matches a known overflow phrase.
    `nonOverflowPatterns` excludes look-alikes such as rate-limit messages.
2.  Some providers (e.g. z.ai) succeed but report usage above the window; pass a
    non-zero `contextWindow` to catch this.
3.  Others (e.g. Xiaomi MiMo) truncate oversized input to fill the window and
    stop on length with zero output.

Passing `contextWindow` as `0` checks error messages only (case 1).

Source: [`llm/transform.go`](https://github.com/ktsoator/or/blob/main/llm/transform.go),
[`llm/overflow.go`](https://github.com/ktsoator/or/blob/main/llm/overflow.go).
