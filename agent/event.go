package agent

import "github.com/ktsoator/or/llm"

// AgentEventType identifies the kind of update emitted while a run progresses.
type AgentEventType string

const (
	// AgentStart marks the beginning of a run.
	AgentStart AgentEventType = "agent_start"
	// AgentEnd is the final event of a run; it carries the appended messages.
	AgentEnd AgentEventType = "agent_end"
	// StepStart marks the beginning of one assistant step.
	StepStart AgentEventType = "step_start"
	// StepEnd marks the end of a step, after its tool calls are executed.
	StepEnd AgentEventType = "step_end"
	// FollowUpStart marks a queued follow-up becoming the next unit of user
	// intent in the same run. Steering messages do not emit this event.
	FollowUpStart AgentEventType = "follow_up_start"
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
	// ToolUpdate carries non-terminal progress streamed during execution.
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
	// ToolResults are the tool results produced during a step, set on StepEnd.
	ToolResults []llm.ToolResultMessage

	// ToolCallID and ToolName identify the tool on tool execution events.
	ToolCallID string
	ToolName   string
	// Args is the validated tool arguments on tool execution events.
	Args any
	// Progress is populated only on ToolUpdate.
	Progress ToolProgress
	// Result is the terminal result populated only on ToolEnd.
	Result ToolResult
	// IsError is the model-facing compatibility projection of Result.Outcome.
	// ToolEnd consumers should use the outcome as the source of truth.
	IsError bool

	// Messages carries the messages the run appended, set on AgentEnd.
	Messages []AgentMessage
}
