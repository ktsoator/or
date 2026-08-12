package agent

import (
	"context"
	"sync"

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
	// StreamingMessage is the partial message for the response currently
	// streaming, or nil when none is in flight. It updates as deltas arrive and
	// clears when the message completes.
	StreamingMessage AgentMessage
	// PendingToolCalls holds the ids of tool calls currently executing, in the
	// order they started.
	PendingToolCalls []string
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
	// GetAPIKey resolves the provider API key before each turn, for short-lived
	// tokens. A non-empty return overrides the key; nil or "" leaves it unchanged.
	GetAPIKey func(provider string) string
	// SteeringMode and FollowUpMode control how many queued messages are injected
	// at one drain point. The zero value is QueueOneAtATime.
	SteeringMode QueueMode
	FollowUpMode QueueMode
	// StreamFn reaches a model for one turn. A nil value uses llm.Stream. It
	// exists mainly as a seam for tests and custom transports.
	StreamFn StreamFn
	// StreamOptions are the base per-request options passed to the stream
	// function on every turn, for knobs like Temperature, MaxTokens, Headers, the
	// OnRequest and OnResponse observers, or the RewriteRequest hook. The agent
	// sets Reasoning from ThinkingLevel and resolves APIKey via GetAPIKey, so
	// values in those two fields are ignored here.
	StreamOptions llm.StreamOptions

	BeforeToolCall      func(BeforeToolCallCtx) (block bool, reason string)
	AfterToolCall       func(AfterToolCallCtx) *AfterToolCallResult
	ShouldStopAfterTurn func(TurnCtx) bool
	PrepareNextTurn     func(TurnCtx) *TurnUpdate
}

// Agent is a stateful wrapper over RunLoop. It owns the transcript, fans events
// out to subscribers, and backs the steering and follow-up queues.
//
// Prompt blocks until the run completes; call it from its own goroutine if you
// want to Steer, FollowUp, or Abort concurrently. All methods are safe for
// concurrent use.
type Agent struct {
	mu               sync.Mutex
	systemPrompt     string
	model            llm.Model
	thinkingLevel    llm.ModelThinkingLevel
	tools            []AgentTool
	messages         []AgentMessage
	isStreaming      bool
	streamingMessage AgentMessage
	pendingToolCalls []string
	errorMessage     string
	cancel           context.CancelFunc

	convertToLLM        func([]AgentMessage) []llm.Message
	transformContext    func([]AgentMessage) []AgentMessage
	toolExecution       ExecutionMode
	getAPIKey           func(provider string) string
	streamFn            StreamFn
	streamOptions       llm.StreamOptions
	beforeToolCall      func(BeforeToolCallCtx) (bool, string)
	afterToolCall       func(AfterToolCallCtx) *AfterToolCallResult
	shouldStopAfterTurn func(TurnCtx) bool
	prepareNextTurn     func(TurnCtx) *TurnUpdate

	steering *messageQueue
	followUp *messageQueue

	listeners      map[int]func(AgentEvent)
	nextListenerID int
}

// New creates an Agent from opts.
func New(opts Options) *Agent {
	return &Agent{
		systemPrompt:        opts.SystemPrompt,
		model:               opts.Model,
		thinkingLevel:       opts.ThinkingLevel,
		tools:               append([]AgentTool(nil), opts.Tools...),
		messages:            append([]AgentMessage(nil), opts.Messages...),
		convertToLLM:        opts.ConvertToLLM,
		transformContext:    opts.TransformContext,
		toolExecution:       opts.ToolExecution,
		getAPIKey:           opts.GetAPIKey,
		streamFn:            opts.StreamFn,
		streamOptions:       opts.StreamOptions,
		beforeToolCall:      opts.BeforeToolCall,
		afterToolCall:       opts.AfterToolCall,
		shouldStopAfterTurn: opts.ShouldStopAfterTurn,
		prepareNextTurn:     opts.PrepareNextTurn,
		steering:            &messageQueue{mode: queueModeOrDefault(opts.SteeringMode)},
		followUp:            &messageQueue{mode: queueModeOrDefault(opts.FollowUpMode)},
		listeners:           make(map[int]func(AgentEvent)),
	}
}
