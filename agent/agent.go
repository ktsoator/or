package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ktsoator/or/llm"
)

// errBusy is returned by Prompt when a run is already in progress.
var errBusy = errors.New("agent: a prompt is already in progress")

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
	// StreamFn reaches a model for one turn. A nil value uses llm.Stream. It
	// exists mainly as a seam for tests and custom transports.
	StreamFn StreamFn

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
	mu            sync.Mutex
	systemPrompt  string
	model         llm.Model
	thinkingLevel llm.ModelThinkingLevel
	tools         []AgentTool
	messages      []AgentMessage
	isStreaming   bool
	errorMessage  string
	cancel        context.CancelFunc

	convertToLLM        func([]AgentMessage) []llm.Message
	transformContext    func([]AgentMessage) []AgentMessage
	toolExecution       ExecutionMode
	streamFn            StreamFn
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
		streamFn:            opts.StreamFn,
		beforeToolCall:      opts.BeforeToolCall,
		afterToolCall:       opts.AfterToolCall,
		shouldStopAfterTurn: opts.ShouldStopAfterTurn,
		prepareNextTurn:     opts.PrepareNextTurn,
		steering:            &messageQueue{},
		followUp:            &messageQueue{},
		listeners:           make(map[int]func(AgentEvent)),
	}
}

// Prompt starts a run from a text string, a single AgentMessage, or a slice of
// them, and blocks until the run completes. It appends the run's messages to the
// transcript and returns an error if the run ended in failure or cancellation.
// Calling Prompt while a run is in progress returns an error.
func (a *Agent) Prompt(ctx context.Context, input any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	prompts, err := toPrompts(input)
	if err != nil {
		return err
	}

	a.mu.Lock()
	if a.isStreaming {
		a.mu.Unlock()
		return errBusy
	}
	a.isStreaming = true
	a.errorMessage = ""
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	base := Context{
		SystemPrompt: a.systemPrompt,
		Messages:     append([]AgentMessage(nil), a.messages...),
		Tools:        append([]AgentTool(nil), a.tools...),
	}
	cfg := a.loopConfigLocked()
	a.mu.Unlock()

	defer cancel()

	var appended []AgentMessage
	for event := range RunLoop(runCtx, prompts, base, cfg) {
		if event.Type == AgentEnd {
			appended = event.Messages
		}
		a.dispatch(event)
	}

	errText := lastAssistantError(appended)

	a.mu.Lock()
	a.messages = append(a.messages, appended...)
	a.isStreaming = false
	a.cancel = nil
	a.errorMessage = errText
	a.mu.Unlock()

	if errText != "" {
		return errors.New(errText)
	}
	return nil
}

// Subscribe registers a listener for run events and returns a function that
// removes it. Listeners are called synchronously from the goroutine running
// Prompt, in event order.
func (a *Agent) Subscribe(listener func(AgentEvent)) (unsubscribe func()) {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := a.nextListenerID
	a.nextListenerID++
	a.listeners[id] = listener

	return func() {
		a.mu.Lock()
		delete(a.listeners, id)
		a.mu.Unlock()
	}
}

// Steer queues a message to inject into the current run before its next turn.
func (a *Agent) Steer(message AgentMessage) {
	a.steering.enqueue(message)
}

// FollowUp queues a message to process after the current run would stop.
func (a *Agent) FollowUp(message AgentMessage) {
	a.followUp.enqueue(message)
}

// Abort cancels the current run, if any.
func (a *Agent) Abort() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Snapshot returns a read-only view of the agent's current state.
func (a *Agent) Snapshot() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return State{
		SystemPrompt:  a.systemPrompt,
		Model:         a.model,
		ThinkingLevel: a.thinkingLevel,
		Tools:         append([]AgentTool(nil), a.tools...),
		Messages:      append([]AgentMessage(nil), a.messages...),
		IsStreaming:   a.isStreaming,
		ErrorMessage:  a.errorMessage,
	}
}

// loopConfigLocked builds the LoopConfig for one run. The caller holds a.mu.
func (a *Agent) loopConfigLocked() LoopConfig {
	return LoopConfig{
		Model:               a.model,
		StreamOptions:       llm.StreamOptions{Reasoning: a.thinkingLevel},
		StreamFn:            a.streamFn,
		ConvertToLLM:        a.convertToLLM,
		TransformContext:    a.transformContext,
		ToolExecution:       a.toolExecution,
		BeforeToolCall:      a.beforeToolCall,
		AfterToolCall:       a.afterToolCall,
		ShouldStopAfterTurn: a.shouldStopAfterTurn,
		PrepareNextTurn:     a.prepareNextTurn,
		GetSteeringMessages: a.steering.drainAll,
		GetFollowUpMessages: a.followUp.drainAll,
	}
}

// dispatch snapshots the listeners under the lock and calls them outside it, so
// a listener may call back into the agent without deadlocking.
func (a *Agent) dispatch(event AgentEvent) {
	a.mu.Lock()
	listeners := make([]func(AgentEvent), 0, len(a.listeners))
	for _, listener := range a.listeners {
		listeners = append(listeners, listener)
	}
	a.mu.Unlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// messageQueue is a concurrency-safe FIFO backing the steering and follow-up
// queues.
type messageQueue struct {
	mu    sync.Mutex
	items []AgentMessage
}

func (q *messageQueue) enqueue(message AgentMessage) {
	q.mu.Lock()
	q.items = append(q.items, message)
	q.mu.Unlock()
}

func (q *messageQueue) drainAll() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	drained := q.items
	q.items = nil
	return drained
}

// toPrompts normalizes Prompt input into messages.
func toPrompts(input any) ([]AgentMessage, error) {
	switch value := input.(type) {
	case string:
		return []AgentMessage{FromLLM(&llm.UserMessage{
			Content: []llm.UserContent{&llm.TextContent{Text: value}},
		})}, nil
	case AgentMessage:
		return []AgentMessage{value}, nil
	case []AgentMessage:
		if len(value) == 0 {
			return nil, errors.New("agent: prompt input is empty")
		}
		return append([]AgentMessage(nil), value...), nil
	case nil:
		return nil, errors.New("agent: prompt input is nil")
	default:
		return nil, fmt.Errorf("agent: unsupported prompt input type %T", input)
	}
}

// lastAssistantError returns the error text of the run's final assistant turn
// when it failed or was aborted, or "" otherwise.
func lastAssistantError(messages []AgentMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		assistant, ok := assistantMessage(messages[index])
		if !ok {
			continue
		}
		if assistant.StopReason == llm.StopReasonError || assistant.StopReason == llm.StopReasonAborted {
			if assistant.ErrorMessage != "" {
				return assistant.ErrorMessage
			}
			return string(assistant.StopReason)
		}
		return ""
	}
	return ""
}

// assistantMessage unwraps an AgentMessage into an llm assistant message.
func assistantMessage(message AgentMessage) (*llm.AssistantMessage, bool) {
	wrapped, ok := message.(llmMessage)
	if !ok {
		return nil, false
	}
	assistant, ok := wrapped.Message.(*llm.AssistantMessage)
	return assistant, ok
}
