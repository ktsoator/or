# Streaming internals

Streaming is normalized around content blocks. A provider may stream JSON chunks,
SSE events, or SDK-specific unions, but the package reports the same lifecycle:
a stream starts, each content block starts and receives deltas, each block ends,
and exactly one terminal event closes the channel.

## Event lifecycle

| Content | Start | Delta | End |
|---|---|---|---|
| Text | `EventTextStart` | `EventTextDelta` | `EventTextEnd` |
| Reasoning | `EventThinkingStart` | `EventThinkingDelta` | `EventThinkingEnd` |
| Tool call | `EventToolCallStart` | `EventToolCallDelta` | `EventToolCallEnd` |

Every non-terminal event carries `Partial`, a cloned snapshot of the
`AssistantMessage` assembled so far. The terminal event is either `EventDone`
with the final message or `EventError` with a partial message and error.

## StreamWriter

Provider adapters do not send directly to the channel. They build an
`AssistantMessage` in memory and pass a pointer to `NewStreamWriter`. The writer
then owns the event-channel invariants:

- `Start()` is idempotent and emits exactly one `EventStart`.
- `Emit()` attaches a fresh `Partial` snapshot to each non-terminal event.
- `Done()` emits one `EventDone`, unless the context was cancelled.
- `Fail()` emits one `EventError`.

The writer guards all of this with a mutex and a `finished` flag, so a late
provider error cannot send a second terminal event. Context cancellation is
reported as `StopReasonAborted`; other failures become `StopReasonError`.

## Provider state

Each adapter keeps its own `streamState` for protocol quirks. The OpenAI adapter
tracks tool calls by both stream index and ID, because compatible providers vary
in which one they repeat across chunks. The Anthropic adapter tracks content
blocks by the provider's stream index and records whether a stop signal arrived,
so a clean socket close without a stop event is treated as an error.

## Tool arguments

Tool-call arguments stream as raw JSON fragments. The final `EventToolCallEnd`
parses the accumulated string with `ParseToolArgumentsMode`, which can repair bad
escapes or close a truncated object. Recovered arguments do not fail the whole
response; instead, the final `AssistantMessage.Diagnostics` records the recovery
mode so a caller can decline to execute unsafe tool calls.

Source: [`llm/stream.go`](https://github.com/ktsoator/or/blob/main/llm/stream.go),
[`llm/events.go`](https://github.com/ktsoator/or/blob/main/llm/events.go).
