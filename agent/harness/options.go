package harness

import (
	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// Options configures a Harness. The fields mirror the subset of agent.Options a
// harness drives, plus the subsystem hooks. A zero value beyond Model is a plain
// agent run with no persistence.
type Options struct {
	// Model is the model used for turns. Required.
	Model llm.Model
	// SystemPrompt is the static system prompt, used when BuildSystemPrompt is
	// nil.
	SystemPrompt string
	// BuildSystemPrompt, when set, builds the system prompt before every turn and
	// takes precedence over SystemPrompt. Use it for prompts that depend on the
	// model or conversation state.
	BuildSystemPrompt SystemPromptFunc
	// ThinkingLevel sets the reasoning effort for each turn.
	ThinkingLevel llm.ModelThinkingLevel
	// Tools is the full tool registry. The model is advertised the active subset
	// (see ActiveTools and SetActiveTools).
	Tools []agent.AgentTool
	// ActiveTools restricts which registered tools are advertised initially, by
	// name. A nil or empty slice activates every tool in Tools.
	ActiveTools []string
	// ToolExecution is the default batch execution mode. Empty means parallel.
	ToolExecution agent.ExecutionMode

	// StreamOptions are the base per-request options for every turn.
	StreamOptions llm.StreamOptions
	// StreamFn reaches a model for one turn. A nil value uses llm.Stream; it
	// exists mainly as a seam for tests and custom transports.
	StreamFn agent.StreamFn
	// ConvertToLLM projects the transcript into llm messages for one request. A
	// nil value uses the agent default.
	ConvertToLLM func([]agent.AgentMessage) []llm.Message
	// GetAPIKey resolves the provider API key before each turn, for short-lived
	// tokens. A non-empty return overrides StreamOptions.APIKey.
	GetAPIKey func(provider string) string

	// SteeringMode and FollowUpMode control how many queued messages are injected
	// at one drain point. The zero value is agent.QueueOneAtATime.
	SteeringMode agent.QueueMode
	FollowUpMode agent.QueueMode

	// Session persists the transcript and seeds it on construction. A nil Session
	// disables persistence.
	Session Session
	// Compactor shrinks the transcript projected to the model each turn. A nil
	// Compactor disables compaction.
	Compactor Compactor
}
