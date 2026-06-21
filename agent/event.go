package agent

import "github.com/ktsoator/or/llm"

// AgentEventType identifies the kind of update emitted while a run progresses.
type AgentEventType string

const (
	// AgentStart marks the beginning of a run.
	AgentStart AgentEventType = "agent_start"
	// AgentEnd is the final event of a run; it carries the appended messages.
	AgentEnd AgentEventType = "agent_end"
	// TurnStart marks the beginning of one assistant turn.
	TurnStart AgentEventType = "turn_start"
	// TurnEnd marks the end of a turn, after its tool calls are executed.
	TurnEnd AgentEventType = "turn_end"
	// MessageStart marks a user, assistant, or tool-result message entering the
	// transcript.
	MessageStart AgentEventType = "message_start"
	// MessageUpdate carries an incremental assistant update; LLMEvent is the
	// underlying llm.Event passed through.
	MessageUpdate AgentEventType = "message_update"
	// MessageEnd marks a completed message.
	MessageEnd AgentEventType = "message_end"
	// ToolStart marks the start of a tool execution.
	ToolStart AgentEventType = "tool_execution_start"
	// ToolUpdate carries a partial tool result streamed during execution.
	ToolUpdate AgentEventType = "tool_execution_update"
	// ToolEnd marks a finished tool execution.
	ToolEnd AgentEventType = "tool_execution_end"
)

// AgentEvent is a single update emitted while running an agent. Fields are
// populated according to Type; unrelated fields are left zero.
type AgentEvent struct {
	Type AgentEventType

	// Message is the message a lifecycle event refers to.
	Message AgentMessage
	// LLMEvent is the underlying llm event, set on MessageUpdate.
	LLMEvent *llm.Event

	// ToolCallID and ToolName identify the tool on tool execution events.
	ToolCallID string
	ToolName   string
	// Args is the validated tool arguments on tool execution events.
	Args any
	// Result is the (possibly partial) tool result on tool execution events.
	Result any
	// IsError reports whether a finished tool result is an error.
	IsError bool

	// Messages carries the messages the run appended, set on AgentEnd.
	Messages []AgentMessage
}
