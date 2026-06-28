# Switching models

The canonical conversation history is provider-neutral, but not every model can
accept every block exactly as stored. `TransformMessages` projects the stored
history into a request-safe form for the target model before an adapter converts
it to wire format.

The important detail: transformation happens per request. It does not mutate the
stored history, so switching to a text-only model does not permanently discard
images, and switching away from a reasoning model does not erase its signatures.

## Transformation order

`TransformMessages` applies three passes:

1. `downgradeUnsupportedImages` replaces image blocks with placeholder text when
   the target model does not list `Image` in `Model.Input`.
2. `reconcileAssistantHistory` rewrites prior assistant turns for the target
   model, including reasoning blocks and tool-call IDs.
3. `synthesizeOrphanedToolResults` repairs transcripts where an assistant tool
   call has no matching tool result.

## Reasoning and signatures

Reasoning is model-specific. A reasoning signature or redacted payload is only
safe to replay when provider, protocol, and model ID all match the target model.
For the same model, signed reasoning is preserved. Across models, readable
thinking is downgraded to plain text, empty thinking is dropped, and redacted
thinking is removed because only the original model can understand it.

Text and tool-call signatures follow the same principle: opaque provider
metadata is kept only for the model that produced it.

## Tool-call identifiers

Different protocols accept different tool-call ID shapes. When an assistant turn
crosses models, the adapter supplies a `normalizeToolCallID` function. If the ID
changes, the matching `ToolResultMessage.ToolCallID` is remapped later in the
same pass, so the transcript stays consistent.

The final pass enforces the tool protocol invariant: every assistant tool-call
batch must receive one result per call before another user or assistant turn.
Missing results are synthesized as error tool results with `"No result provided"`.

## Overflow detection

`IsContextOverflow` is separate from transformation. It inspects a completed or
failed `AssistantMessage` and detects provider-specific context overflow signals:
known error-message patterns, successful responses whose usage exceeds the
context window, and length stops where input filled almost the whole window and
no output was produced.

Source: [`llm/transform.go`](https://github.com/ktsoator/or/blob/main/llm/transform.go),
[`llm/overflow.go`](https://github.com/ktsoator/or/blob/main/llm/overflow.go).
