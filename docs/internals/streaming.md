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

A stream opens with one `EventStart` and closes with exactly one terminal event —
`EventDone` on success or `EventError` on failure.

## The Event union

`Event` is a flat union: `Type` selects the kind of update, and only a subset of
the fields is meaningful for each `Type`.

```go
type Event struct {
	// Type selects which of the fields below are meaningful; see the table above.
	Type EventType

	// ContentIndex is the position of the affected block within the assembled
	// message content, on the per-block start/delta/end events.
	ContentIndex int

	// Delta is newly streamed text on a *Delta event, or a fragment of argument
	// JSON on EventToolCallDelta.
	Delta string

	// Content is the completed block text on EventTextEnd and EventThinkingEnd.
	Content string

	// ToolCall is the tool call being assembled, on the toolcall events. It holds
	// the best-effort parsed call at EventToolCallEnd.
	ToolCall *ToolCall

	// Partial is a snapshot of the message assembled so far, on every non-terminal
	// event.
	Partial *AssistantMessage

	// Message is the final assistant message, on the terminal EventDone and
	// EventError events.
	Message *AssistantMessage

	// Err is the stream failure, on EventError.
	Err error
}
```

Which fields are populated is fixed per `Type`. Reading a field not listed for
the current `Type` returns a zero value that means nothing:

| Type | Meaningful fields (besides `Type`) |
|---|---|
| `EventStart` | `Partial` |
| `EventTextStart` | `ContentIndex`, `Partial` |
| `EventTextDelta` | `ContentIndex`, `Delta`, `Partial` |
| `EventTextEnd` | `ContentIndex`, `Content`, `Partial` |
| `EventThinkingStart` | `ContentIndex`, `Partial` |
| `EventThinkingDelta` | `ContentIndex`, `Delta`, `Partial` |
| `EventThinkingEnd` | `ContentIndex`, `Content`, `Partial` |
| `EventToolCallStart` | `ContentIndex`, `ToolCall`, `Partial` |
| `EventToolCallDelta` | `ContentIndex`, `Delta`, `ToolCall`, `Partial` |
| `EventToolCallEnd` | `ContentIndex`, `ToolCall`, `Partial` |
| `EventDone` | `Message` |
| `EventError` | `Message`, `Err` |

`Partial` is attached to every non-terminal event; the terminal events carry the
final `Message` instead.

## StreamWriter

Provider adapters do not send to the channel directly. They build an
`AssistantMessage` in memory, hand a pointer to `NewStreamWriter`, and drive it
with `Emit`, `Done`, and `Fail`. The writer owns the channel invariants: one
`EventStart`, a `Partial` snapshot on every non-terminal event, and exactly one
terminal event.

`Emit` first emits `EventStart` if needed, then clones the message built so far
into `Partial` so each event is an independent snapshot:

```go
func (w *StreamWriter) Emit(event Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished { // (1)!
		return
	}
	w.startLocked()
	event.Partial = cloneAssistantMessage(*w.output) // (2)!
	w.events <- event
}
```

1.  Once a terminal event has been sent, every later call is a no-op — this is
    what makes the single-terminal guarantee hold.
2.  A deep clone, so a consumer holding an earlier `Partial` never sees it mutate
    as the stream continues.

`Done` normally sends `EventDone`, but a cancelled context is redirected to the
failure path so cancellation always surfaces as an error, never a success:

```go
func (w *StreamWriter) Done() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.startLocked()
	if err := w.ctx.Err(); err != nil { // (1)!
		w.failLocked(err)
		return
	}
	w.finished = true
	w.events <- Event{Type: EventDone, Message: cloneAssistantMessage(*w.output)}
}
```

1.  A stream that assembled a full message but was cancelled ends as an error, so
    a caller cannot mistake an aborted turn for a completed one.

`failLocked` sets the stop reason and error, distinguishing a cancelled context
from an ordinary failure:

```go
func (w *StreamWriter) failLocked(err error) {
	w.finished = true
	if err == nil {
		err = errors.New("stream failed")
	}
	output := *w.output
	if w.ctx.Err() != nil {
		output.StopReason = StopReasonAborted // (1)!
		err = w.ctx.Err()
	} else {
		output.StopReason = StopReasonError
	}
	output.ErrorMessage = err.Error()
	w.events <- Event{Type: EventError, Message: cloneAssistantMessage(output), Err: err}
}
```

1.  Cancellation is reported as `StopReasonAborted` and the error is replaced with
    the context error; any other failure is `StopReasonError`.

The mutex plus the `finished` flag mean a late provider error arriving after
`Done` cannot send a second terminal event.

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
response; instead the final `AssistantMessage.Diagnostics` records the recovery
mode, so a caller can wait for `EventDone` and decline to execute a call whose
arguments were only partially recovered.

Source: [`llm/stream.go`](https://github.com/ktsoator/or/blob/main/llm/stream.go),
[`llm/events.go`](https://github.com/ktsoator/or/blob/main/llm/events.go).
