package llm

// EventType identifies the kind of update emitted by a provider stream.
type EventType string

const (
	// EventStart marks the beginning of a provider stream.
	EventStart EventType = "start"
	// EventTextStart marks the creation of a text content block.
	EventTextStart EventType = "text_start"
	// EventTextDelta carries newly generated text.
	EventTextDelta EventType = "text_delta"
	// EventTextEnd carries the completed text content block.
	EventTextEnd EventType = "text_end"
	// EventThinkingStart marks the creation of a reasoning content block.
	EventThinkingStart EventType = "thinking_start"
	// EventThinkingDelta carries newly generated reasoning content.
	EventThinkingDelta EventType = "thinking_delta"
	// EventThinkingEnd carries the completed reasoning content block.
	EventThinkingEnd EventType = "thinking_end"
	// EventToolCallStart marks the creation of a tool call content block.
	EventToolCallStart EventType = "toolcall_start"
	// EventToolCallDelta carries a fragment of a tool call's arguments as it streams.
	EventToolCallDelta EventType = "toolcall_delta"
	// EventToolCallEnd carries a tool call whose arguments finished streaming.
	// Malformed or truncated argument JSON is parsed best-effort, so callers
	// should validate the arguments and wait for EventDone before executing the
	// call, because a later content block may still fail the overall response.
	EventToolCallEnd EventType = "toolcall_end"
	// EventDone carries the final assistant message.
	EventDone EventType = "done"
	// EventError carries a stream failure.
	EventError EventType = "error"
)

// Event is a single update emitted while streaming a provider response.
//
// It is a flat union: Type selects the kind of update, and only a subset of the
// fields is meaningful for each Type. Fields not listed for a Type are zero and
// must not be read. The valid combinations are fixed:
//
//	Type                 Meaningful fields (besides Type)
//	-------------------  -------------------------------------------
//	EventStart           Partial
//	EventTextStart       ContentIndex, Partial
//	EventTextDelta       ContentIndex, Delta, Partial
//	EventTextEnd         ContentIndex, Content, Partial
//	EventThinkingStart   ContentIndex, Partial
//	EventThinkingDelta   ContentIndex, Delta, Partial
//	EventThinkingEnd     ContentIndex, Content, Partial
//	EventToolCallStart   ContentIndex, ToolCall, Partial
//	EventToolCallDelta   ContentIndex, Delta, ToolCall, Partial
//	EventToolCallEnd     ContentIndex, ToolCall, Partial
//	EventDone            Message
//	EventError           Message, Err
//
// Partial is attached to every non-terminal event; the terminal events
// (EventDone, EventError) carry the final Message instead of a Partial.
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
