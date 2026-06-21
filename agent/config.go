package agent

import "github.com/ktsoator/or/llm"

// Context is the starting transcript, system prompt, and tools for a run.
type Context struct {
	SystemPrompt string
	Messages     []AgentMessage
	Tools        []AgentTool
}

// TurnCtx is passed to the per-turn hooks after a turn's assistant message and
// tool results have been appended.
type TurnCtx struct {
	// Message is the assistant message that completed the turn.
	Message llm.AssistantMessage
	// ToolResults are the tool results produced during the turn.
	ToolResults []llm.ToolResultMessage
	// Context is the current run context.
	Context Context
	// NewMessages are the messages this run would return if it exits now.
	NewMessages []AgentMessage
}

// TurnUpdate replaces runtime state before the next turn. Nil fields keep the
// current value.
type TurnUpdate struct {
	Context       *Context
	Model         *llm.Model
	ThinkingLevel *llm.ModelThinkingLevel
}

// BeforeToolCallCtx is passed to BeforeToolCall, after arguments validate and
// before the tool executes.
type BeforeToolCallCtx struct {
	AssistantMessage llm.AssistantMessage
	ToolCall         llm.ToolCall
	Args             any
	Context          Context
}

// AfterToolCallCtx is passed to AfterToolCall, after the tool executes and
// before its events are emitted.
type AfterToolCallCtx struct {
	AssistantMessage llm.AssistantMessage
	ToolCall         llm.ToolCall
	Args             any
	Result           ToolResult
	IsError          bool
	Context          Context
}

// AfterToolCallResult overrides parts of an executed tool result. Each field
// overrides the corresponding result value only when set; a nil field keeps the
// original. Content replaces the whole content slice when non-nil; Details
// replaces the details when non-nil.
type AfterToolCallResult struct {
	Content   []llm.ToolResultContent
	Details   any
	IsError   *bool
	Terminate *bool
}

// LoopConfig configures a run. All extension points are function fields; the
// zero value of each is "no hook". Given a Model and ConvertToLLM, the zero
// config is a plain tool loop with no interception.
type LoopConfig struct {
	// Model is the model used for turns until PrepareNextTurn replaces it.
	Model llm.Model
	// StreamOptions are the per-request options passed to the stream function.
	StreamOptions llm.StreamOptions
	// StreamFn reaches a model for one turn. A nil value uses llm.Stream. It
	// exists mainly as a seam for tests and custom transports.
	StreamFn StreamFn
	// ConvertToLLM projects the transcript into llm.Message values for one
	// request. A nil value uses the default, which unwraps FromLLM messages and
	// drops everything else.
	ConvertToLLM func([]AgentMessage) []llm.Message
	// ToolExecution is the default batch execution mode. Empty means parallel.
	ToolExecution ExecutionMode

	// BeforeToolCall runs after arguments validate and before execution.
	// Returning block=true skips the tool; reason becomes the error result text.
	BeforeToolCall func(BeforeToolCallCtx) (block bool, reason string)

	// AfterToolCall runs after a tool finishes. A non-nil return overrides the
	// executed result field by field; nil keeps it unchanged.
	AfterToolCall func(AfterToolCallCtx) *AfterToolCallResult

	// ShouldStopAfterTurn requests a graceful stop after the current turn,
	// before another model request starts.
	ShouldStopAfterTurn func(TurnCtx) bool

	// PrepareNextTurn may replace the model, thinking level, or context for the
	// next turn. Returning nil keeps the current settings.
	PrepareNextTurn func(TurnCtx) *TurnUpdate

	// TransformContext adjusts the transcript before projection. It is the
	// attachment point for context compaction; phase one ships no default.
	TransformContext func([]AgentMessage) []AgentMessage

	// GetSteeringMessages is polled after each turn's tool calls finish, to
	// inject messages mid-run.
	GetSteeringMessages func() []AgentMessage
	// GetFollowUpMessages is polled when the agent would otherwise stop, to
	// continue it with another turn.
	GetFollowUpMessages func() []AgentMessage
}
