package llm

import (
	"context"
	"errors"
	"sync"
)

// StreamWriter manages the event channel for one streamed response. It emits a
// single EventStart, attaches a Partial snapshot to every non-terminal event,
// and guarantees exactly one terminal event (EventDone or EventError) before the
// channel closes. A cancelled context is reported as StopReasonAborted.
//
// A protocol adapter builds its AssistantMessage incrementally — the writer is
// constructed over a pointer to it — emits deltas through Emit, and ends with
// Done on success or Fail on error. This is the shared machinery behind the
// built-in adapters; it keeps the single-terminal guarantee and Partial cloning
// in one place rather than duplicated per provider.
type StreamWriter struct {
	ctx      context.Context
	events   chan<- Event
	output   *AssistantMessage
	mu       sync.Mutex
	started  bool
	finished bool
}

// NewStreamWriter returns a writer that sends events to the channel and snapshots
// output for each event's Partial and for the terminal Message.
func NewStreamWriter(ctx context.Context, events chan<- Event, output *AssistantMessage) *StreamWriter {
	return &StreamWriter{ctx: ctx, events: events, output: output}
}

// Start emits EventStart. It is idempotent: only the first call emits.
func (w *StreamWriter) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.startLocked()
}

func (w *StreamWriter) startLocked() {
	if w.started {
		return
	}
	w.started = true
	w.events <- Event{Type: EventStart, Partial: cloneAssistantMessage(*w.output)}
}

// Emit sends a non-terminal event, first emitting EventStart if needed and
// attaching a fresh Partial snapshot of the message built so far.
func (w *StreamWriter) Emit(event Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.startLocked()
	event.Partial = cloneAssistantMessage(*w.output)
	w.events <- event
}

// Done emits the single terminal EventDone carrying the final message.
func (w *StreamWriter) Done() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.startLocked()
	if err := w.ctx.Err(); err != nil {
		w.failLocked(err)
		return
	}
	w.finished = true
	w.events <- Event{Type: EventDone, Message: cloneAssistantMessage(*w.output)}
}

// Fail emits the single terminal EventError. A cancelled context is reported as
// StopReasonAborted and replaces the error with the context error; any other
// failure is reported as StopReasonError.
func (w *StreamWriter) Fail(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return
	}
	w.startLocked()
	w.failLocked(err)
}

func (w *StreamWriter) failLocked(err error) {
	w.finished = true
	if err == nil {
		err = errors.New("stream failed")
	}
	output := *w.output
	if w.ctx.Err() != nil {
		output.StopReason = StopReasonAborted
		err = w.ctx.Err()
	} else {
		output.StopReason = StopReasonError
	}
	output.ErrorMessage = err.Error()
	w.events <- Event{Type: EventError, Message: cloneAssistantMessage(output), Err: err}
}

// CloneToolCall returns a deep copy of a tool call, including its arguments, for
// use in an event's ToolCall field.
func CloneToolCall(toolCall *ToolCall) *ToolCall {
	if toolCall == nil {
		return nil
	}
	clone := *toolCall
	clone.Arguments = cloneJSONObject(toolCall.Arguments)
	return &clone
}

func cloneAssistantMessage(message AssistantMessage) *AssistantMessage {
	clone := message
	clone.Content = make([]AssistantContent, len(message.Content))
	for i, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *TextContent:
			if content != nil {
				copied := *content
				clone.Content[i] = &copied
			}
		case *ThinkingContent:
			if content != nil {
				copied := *content
				clone.Content[i] = &copied
			}
		case *ToolCall:
			clone.Content[i] = CloneToolCall(content)
		}
	}
	if len(message.Diagnostics) > 0 {
		clone.Diagnostics = append([]Diagnostic(nil), message.Diagnostics...)
	}
	return &clone
}

func cloneJSONObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = cloneJSONValue(item)
	}
	return clone
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONObject(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneJSONValue(item)
		}
		return clone
	default:
		return value
	}
}
