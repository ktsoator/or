package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// ErrBusy is returned by Prompt and Continue when a run is already in progress.
// Steer and FollowUp are the way to inject messages into a running agent.
var ErrBusy = errors.New("harness: a run is already in progress")

// Harness is a stateful orchestrator over agent.Agent. It owns the wrapped
// agent and, when configured, persists the transcript to a Session.
//
// Prompt and Continue block until the run completes and are mutually exclusive;
// a concurrent call returns ErrBusy. Steer, FollowUp, Abort, Subscribe, and
// Snapshot are safe to call while a run is in progress.
type Harness struct {
	agent       *agent.Agent
	session     Session
	buildPrompt SystemPromptFunc
	compactor   Compactor

	// runMu is held for the duration of a Prompt or Continue run. It serializes
	// runs, guards persistedLen (which only changes after a run completes), and
	// makes runCtx stable for the compaction hook during a run.
	runMu        sync.Mutex
	persistedLen int
	runCtx       context.Context
}

// New builds a Harness. When a Session is configured, its stored transcript is
// loaded and used to seed the agent, so a new Harness resumes where the last one
// left off.
func New(ctx context.Context, opts Options) (*Harness, error) {
	var seed []agent.AgentMessage
	if opts.Session != nil {
		loaded, err := opts.Session.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("harness: load session: %w", err)
		}
		seed = loaded
	}

	h := &Harness{
		session:      opts.Session,
		buildPrompt:  opts.BuildSystemPrompt,
		compactor:    opts.Compactor,
		persistedLen: len(seed),
	}

	agentOpts := agent.Options{
		SystemPrompt:  opts.SystemPrompt,
		Model:         opts.Model,
		ThinkingLevel: opts.ThinkingLevel,
		Tools:         opts.Tools,
		Messages:      seed,
		ConvertToLLM:  opts.ConvertToLLM,
		ToolExecution: opts.ToolExecution,
		GetAPIKey:     opts.GetAPIKey,
		SteeringMode:  opts.SteeringMode,
		FollowUpMode:  opts.FollowUpMode,
		StreamFn:      opts.StreamFn,
		StreamOptions: opts.StreamOptions,
	}
	// The builder rebuilds the prompt before each later turn; the first turn is
	// seeded in run() just before the loop starts.
	if h.buildPrompt != nil {
		agentOpts.PrepareNextTurn = h.prepareNextTurn
	}
	if h.compactor != nil {
		agentOpts.TransformContext = h.transformContext
	}
	h.agent = agent.New(agentOpts)

	return h, nil
}

// transformContext runs the configured Compactor over the transcript before each
// turn's request. On error it keeps the full transcript so the run proceeds
// uncompacted rather than failing.
func (h *Harness) transformContext(messages []agent.AgentMessage) []agent.AgentMessage {
	ctx := h.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	compacted, err := h.compactor.Compact(ctx, messages)
	if err != nil {
		return messages
	}
	return compacted
}

// prepareNextTurn rebuilds the system prompt for the turn that follows the one
// just completed, from the live transcript and the agent's current model.
func (h *Harness) prepareNextTurn(turn agent.TurnCtx) *agent.TurnUpdate {
	snapshot := h.agent.Snapshot()
	info := TurnInfo{
		Model:         snapshot.Model,
		ThinkingLevel: snapshot.ThinkingLevel,
		Tools:         turn.Context.Tools,
		Messages:      turn.Context.Messages,
	}
	next := turn.Context
	next.SystemPrompt = h.buildPrompt(info)
	return &agent.TurnUpdate{Context: &next}
}

// applyInitialSystemPrompt builds and sets the system prompt for the first turn
// of a run. Later turns are handled by prepareNextTurn.
func (h *Harness) applyInitialSystemPrompt() {
	if h.buildPrompt == nil {
		return
	}
	snapshot := h.agent.Snapshot()
	info := TurnInfo{
		Model:         snapshot.Model,
		ThinkingLevel: snapshot.ThinkingLevel,
		Tools:         snapshot.Tools,
		Messages:      snapshot.Messages,
	}
	h.agent.SetSystemPrompt(h.buildPrompt(info))
}

// Prompt starts a run from a text message and optional images, blocking until it
// completes. Newly appended messages are persisted to the Session. It returns
// ErrBusy if a run is already in progress.
func (h *Harness) Prompt(ctx context.Context, text string, images ...llm.ImageContent) error {
	if !h.runMu.TryLock() {
		return ErrBusy
	}
	defer h.runMu.Unlock()
	h.runCtx = ctx
	h.applyInitialSystemPrompt()
	runErr := h.agent.Prompt(ctx, agent.UserMessage(text, images...))
	return errors.Join(runErr, h.persistNew(ctx))
}

// Continue resumes a run from the current transcript without adding a message,
// blocking until it completes. It returns ErrBusy if a run is already in
// progress.
func (h *Harness) Continue(ctx context.Context) error {
	if !h.runMu.TryLock() {
		return ErrBusy
	}
	defer h.runMu.Unlock()
	h.runCtx = ctx
	h.applyInitialSystemPrompt()
	runErr := h.agent.Continue(ctx)
	return errors.Join(runErr, h.persistNew(ctx))
}

// persistNew appends the messages added since the last persist to the Session.
// It is called only while runMu is held, so persistedLen is not racing a run.
func (h *Harness) persistNew(ctx context.Context) error {
	if h.session == nil {
		return nil
	}
	all := h.agent.Snapshot().Messages
	if h.persistedLen >= len(all) {
		return nil
	}
	added := all[h.persistedLen:]
	if err := h.session.Append(ctx, added...); err != nil {
		return fmt.Errorf("harness: persist session: %w", err)
	}
	h.persistedLen = len(all)
	return nil
}

// Steer queues a message to inject after the current turn's tool calls finish.
func (h *Harness) Steer(text string, images ...llm.ImageContent) {
	h.agent.Steer(agent.UserMessage(text, images...))
}

// FollowUp queues a message to continue the agent once it would otherwise stop.
func (h *Harness) FollowUp(text string, images ...llm.ImageContent) {
	h.agent.FollowUp(agent.UserMessage(text, images...))
}

// Abort cancels an in-progress run.
func (h *Harness) Abort() { h.agent.Abort() }

// Subscribe registers a listener for run events and returns a function that
// removes it.
func (h *Harness) Subscribe(listener func(agent.AgentEvent)) (unsubscribe func()) {
	return h.agent.Subscribe(listener)
}

// Snapshot returns a read-only snapshot of the underlying agent state.
func (h *Harness) Snapshot() agent.State { return h.agent.Snapshot() }

// Messages returns the current transcript.
func (h *Harness) Messages() []agent.AgentMessage { return h.agent.Snapshot().Messages }

// Agent returns the wrapped agent for advanced callers that need direct access.
func (h *Harness) Agent() *agent.Agent { return h.agent }
