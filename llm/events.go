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
type Event struct {
	Type EventType

	ContentIndex int

	Delta string

	Content string

	ToolCall *ToolCall

	Partial *AssistantMessage

	Message *AssistantMessage

	Err error
}
