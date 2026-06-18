package llm

// EventType identifies the kind of update emitted by a provider stream.
type EventType string

const (
	// EventStart marks the beginning of a provider stream.
	EventStart EventType = "start"
	// EventTextDelta carries newly generated text.
	EventTextDelta EventType = "text_delta"
	// EventThinkingDelta carries newly generated reasoning content.
	EventThinkingDelta EventType = "thinking_delta"
	// EventToolCallEnd carries a completed tool call request.
	EventToolCallEnd EventType = "toolcall_end"
	// EventDone carries the final assistant message.
	EventDone EventType = "done"
	// EventError carries a stream failure.
	EventError EventType = "error"
)

// Event is a single update emitted while streaming a provider response.
type Event struct {
	Type EventType

	Delta string

	ToolCall *ToolCall

	Partial *AssistantMessage

	Message *AssistantMessage

	Err error
}
