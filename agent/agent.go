package agent

import (
	"context"

	"github.com/ktsoator/or/llm"
)

// State is a read-only snapshot of an Agent's runtime state.
type State struct {
	SystemPrompt  string
	Model         llm.Model
	ThinkingLevel llm.ModelThinkingLevel
	Tools         []AgentTool
	Messages      []AgentMessage
	// IsStreaming reports whether a prompt or continuation is in progress.
	IsStreaming bool
	// ErrorMessage holds the error from the most recent failed turn, if any.
	ErrorMessage string
}

// Options configures a new Agent. The hook fields mirror LoopConfig and apply
// to every run the agent drives.
type Options struct {
	SystemPrompt  string
	Model         llm.Model
	ThinkingLevel llm.ModelThinkingLevel
	Tools         []AgentTool
	Messages      []AgentMessage

	ConvertToLLM     func([]AgentMessage) []llm.Message
	TransformContext func([]AgentMessage) []AgentMessage
	ToolExecution    ExecutionMode

	BeforeToolCall      func(BeforeToolCallCtx) (block bool, reason string)
	AfterToolCall       func(AfterToolCallCtx) *ToolResult
	ShouldStopAfterTurn func(TurnCtx) bool
	PrepareNextTurn     func(TurnCtx) *TurnUpdate
}

// Agent is a stateful wrapper over RunLoop. It owns the transcript, fans events
// out to subscribers, and backs the steering and follow-up queues.
type Agent struct {
	// Fields are added during implementation.
}

// New creates an Agent from opts.
//
// Not yet implemented.
func New(opts Options) *Agent {
	panic("agent: New not implemented")
}

// Prompt starts a run from a text string or one or more AgentMessage values.
//
// Not yet implemented.
func (a *Agent) Prompt(ctx context.Context, input any) error {
	panic("agent: Prompt not implemented")
}

// Subscribe registers a listener for run events and returns a function that
// removes it.
//
// Not yet implemented.
func (a *Agent) Subscribe(listener func(AgentEvent)) (unsubscribe func()) {
	panic("agent: Subscribe not implemented")
}

// Steer queues a message to inject into the current run mid-flight.
//
// Not yet implemented.
func (a *Agent) Steer(message AgentMessage) {
	panic("agent: Steer not implemented")
}

// FollowUp queues a message to process after the current run would stop.
//
// Not yet implemented.
func (a *Agent) FollowUp(message AgentMessage) {
	panic("agent: FollowUp not implemented")
}

// Abort cancels the current run.
//
// Not yet implemented.
func (a *Agent) Abort() {
	panic("agent: Abort not implemented")
}

// Snapshot returns a read-only view of the agent's current state.
//
// Not yet implemented.
func (a *Agent) Snapshot() State {
	panic("agent: Snapshot not implemented")
}
