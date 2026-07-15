# Streaming internals

This page explains how adapters implement the public streaming contract. Event names, the field matrix, and the caller consumption rules are maintained only in [LLM streaming](../llm/streaming.md).

A provider may return SSE, JSON fragments, or an SDK union. An adapter builds an in-memory `AssistantMessage` while emitting neutral events through `StreamWriter`. The public layer sees one lifecycle; provider differences stay in each adapter's `streamState`.

## StreamWriter responsibilities

Adapters do not send directly to the event channel. They pass the `AssistantMessage` under construction to `NewStreamWriter`, then call `Emit`, `Done`, or `Fail`.

| Invariant | `StreamWriter` behavior |
|---|---|
| A stream has one start event | The first `Emit`, `Done`, or `Fail` emits `EventStart` when needed |
| Non-terminal events carry independent snapshots | `Emit` deep-clones the current message into `Event.Partial` |
| A stream has one terminal event | A lock and `finished` flag reject later `Done`, `Fail`, and `Emit` calls |
| Cancellation cannot be reported as success | `Done` checks `ctx.Err()` and redirects a cancelled context to failure |
| Failure preserves partial output | `Fail` clones the current message and sets its stop reason and `ErrorMessage` |

`Partial` must be a deep clone. Otherwise, an earlier event retained by a consumer would mutate as the adapter appended content. Cloning also means every delta has an allocation cost; a caller that only needs `Delta` should not repeatedly serialize the full `Partial`.

## Terminal paths

On success, the writer clones the final message into `EventDone.Message`. An ordinary runtime failure produces `EventError` with `StopReasonError`; a cancelled context produces the same event type with `StopReasonAborted` and replaces the error with the context error.

After publishing the terminal event, the writer sets `finished`. A late provider error or content block cannot produce a second terminal event. The writer owns this guarantee so individual adapters do not need to reimplement it.

The event channel is unbuffered. A writer may block while sending, so a caller that stops receiving also prevents adapter cleanup. Context cancellation can stop HTTP work, but it does not replace channel consumption.

## Adapter stream state

Each adapter keeps protocol-specific state and ultimately drives the same writer:

- The OpenAI adapter tracks tool calls by both stream index and tool-call ID because compatible providers differ in which identifier they repeat across chunks.
- The Anthropic adapter assembles blocks by provider content index and records whether a formal stop signal arrived. A clean socket close without that signal is treated as an error.
- An adapter closes the SDK stream when its goroutine exits; the event channel closes after the writer publishes its terminal event.

This state is not exposed to callers. Public code handles results through `Event.Type`, `ContentIndex`, and the terminal message.

## Tool-argument recovery

Tool arguments arrive as raw JSON fragments. The adapter accumulates the string and calls `ParseToolArgumentsMode` when the tool block ends. The parser can repair bad escapes or close a truncated object.

Recovery does not automatically fail the response. The final `AssistantMessage.Diagnostics` records the recovery mode, so the caller should wait for `EventDone` and then decide whether to reject `partial` or `invalid` arguments. See [LLM tools](../llm/tools.md) for execution constraints.

## Resources and concurrency

- One adapter goroutine produces a stream and caller code consumes it.
- The `StreamWriter` lock protects event order and terminal state; it does not buffer the channel.
- `Client` has no `Close`; the adapter closes the provider stream for each request.
- An injected `http.Client` belongs to the application and should be reused across requests.
- Context controls the complete request; `StreamOptions.Timeout` controls one HTTP attempt.

Source: [`llm/stream.go`](https://github.com/ktsoator/or/blob/main/llm/stream.go), [`llm/events.go`](https://github.com/ktsoator/or/blob/main/llm/events.go).
