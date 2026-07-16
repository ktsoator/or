// Package coding is the embeddable core of a coding agent: a stateful session
// built directly on the low-level agent loop. It owns the transcript, wires the
// permission gate and the coding system prompt into the loop, and persists the
// conversation. It has no terminal or CLI dependencies, so it can be embedded as
// a library; the product's shell lives under internal/app.
package coding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/policy"
	"github.com/ktsoator/or/coding/prompt"
	"github.com/ktsoator/or/coding/store"
	"github.com/ktsoator/or/coding/tools"
	"github.com/ktsoator/or/llm"
)

// ErrBusy is returned by Prompt and Continue when a run is already in progress.
// Steer and FollowUp inject messages into a running session instead.
var ErrBusy = errors.New("coding: a run is already in progress")

// Options configures a Session. Only Model is required; the rest have working
// defaults.
type Options struct {
	// Model is the model used for turns. Required.
	Model llm.Model
	// ThinkingLevel sets the reasoning effort for each turn.
	ThinkingLevel llm.ModelThinkingLevel
	// Cwd is the workspace root the tools operate in. Empty uses the process
	// working directory.
	Cwd string
	// Tools is the tool set. Nil uses tools.CodingTools rooted at Cwd, backed by
	// tools.LocalOps.
	Tools []tools.Tool
	// Policy is the permission gate consulted before each tool call. The zero
	// value asks for confirmation on any workspace-changing call; with no Confirm
	// wired, such calls are denied. Set Policy.Confirm to approve them.
	Policy policy.Gate
	// Store persists the transcript and seeds it on construction. Nil disables
	// persistence.
	Store store.Store
	// Instructions overrides the base system-prompt preamble. Empty uses
	// prompt.DefaultInstructions.
	Instructions string
	// MaxRetries caps how many times a transient turn failure is retried above
	// the provider SDK's own request retries. Nil uses defaultMaxRetries; a
	// pointer to 0 disables app-level retries.
	MaxRetries *int

	// StreamOptions are the base per-request options for every turn.
	StreamOptions llm.StreamOptions
	// StreamFn reaches a model for one turn. Nil uses the agent default.
	StreamFn agent.StreamFn
	// GetAPIKey resolves the provider API key before each turn, for short-lived
	// tokens.
	GetAPIKey func(provider string) string
}

// Session is a stateful coding conversation. Prompt and Continue block until a
// run completes and are mutually exclusive; a concurrent call returns ErrBusy.
// Steer, FollowUp, Abort, Subscribe, and Snapshot are safe during a run.
type Session struct {
	agent    *agent.Agent
	store    store.Store
	tools    []tools.Tool
	readOnly map[string]bool
	cwd      string

	maxRetries    int
	contextWindow int64

	runMu        sync.Mutex
	persistedLen int
	runCtx       context.Context
}

// New builds a Session. When a Store is configured, its transcript is loaded and
// used to seed the agent, so the session resumes where it left off.
func New(ctx context.Context, opts Options) (*Session, error) {
	cwd := opts.Cwd
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		cwd = wd
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	toolSet := opts.Tools
	if toolSet == nil {
		toolSet = tools.CodingTools(cwd, tools.LocalOps{})
	}

	var seed []agent.AgentMessage
	if opts.Store != nil {
		loaded, err := opts.Store.Load(ctx)
		if err != nil {
			return nil, err
		}
		seed = loaded
	}

	maxRetries := defaultMaxRetries
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}

	s := &Session{
		store:         opts.Store,
		tools:         toolSet,
		readOnly:      readOnlyNames(toolSet),
		cwd:           cwd,
		maxRetries:    maxRetries,
		contextWindow: opts.Model.ContextWindow,
		persistedLen:  len(seed),
	}

	gate := opts.Policy
	agentOpts := agent.Options{
		SystemPrompt:  s.buildSystemPrompt(opts.Instructions),
		Model:         opts.Model,
		ThinkingLevel: opts.ThinkingLevel,
		Tools:         tools.AgentTools(toolSet),
		Messages:      seed,
		StreamOptions: opts.StreamOptions,
		StreamFn:      opts.StreamFn,
		GetAPIKey:     opts.GetAPIKey,
		BeforeToolCall: func(bc agent.BeforeToolCallCtx) (bool, string) {
			args, _ := bc.Args.(map[string]any)
			return gate.Check(policy.Request{
				Tool:     bc.ToolCall.Name,
				Args:     args,
				ReadOnly: s.readOnly[bc.ToolCall.Name],
			})
		},
	}
	s.agent = agent.New(agentOpts)

	return s, nil
}

// Prompt starts a run from a text message and optional images, blocking until it
// completes. Newly appended messages are persisted. It returns ErrBusy if a run
// is already in progress.
func (s *Session) Prompt(ctx context.Context, text string, images ...llm.ImageContent) error {
	return s.run(ctx, func(ctx context.Context) error {
		return s.agent.Prompt(ctx, agent.UserMessage(text, images...))
	})
}

// Continue resumes a run from the current transcript without adding a message.
// It returns ErrBusy if a run is already in progress.
func (s *Session) Continue(ctx context.Context) error {
	return s.run(ctx, s.agent.Continue)
}

// run serializes a single Prompt or Continue invocation, then persists whatever
// messages it appended.
func (s *Session) run(ctx context.Context, fn func(context.Context) error) error {
	if !s.runMu.TryLock() {
		return ErrBusy
	}
	defer s.runMu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	s.runCtx = ctx
	runErr := fn(ctx)
	if runErr != nil && s.maxRetries > 0 {
		runErr = s.withRetry(ctx, runErr)
	}
	return errors.Join(runErr, s.persistNew(ctx))
}

// persistNew appends the messages added since the last persist to the Store. It
// runs only while runMu is held, so persistedLen is not racing a run.
func (s *Session) persistNew(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	all := s.agent.Snapshot().Messages
	if s.persistedLen >= len(all) {
		return nil
	}
	added := all[s.persistedLen:]
	if err := s.store.Append(ctx, added...); err != nil {
		return err
	}
	s.persistedLen = len(all)
	return nil
}

// Steer queues a message to inject after the current turn's tool calls finish.
func (s *Session) Steer(text string, images ...llm.ImageContent) {
	s.agent.Steer(agent.UserMessage(text, images...))
}

// FollowUp queues a message to process once the run would otherwise stop.
func (s *Session) FollowUp(text string, images ...llm.ImageContent) {
	s.agent.FollowUp(agent.UserMessage(text, images...))
}

// Abort cancels an in-progress run.
func (s *Session) Abort() { s.agent.Abort() }

// Subscribe registers a listener for UI-neutral coding events and returns a
// function that removes it.
func (s *Session) Subscribe(listener func(Event)) (unsubscribe func()) {
	return s.agent.Subscribe(func(ev agent.AgentEvent) {
		if projected, ok := projectAgentEvent(ev); ok {
			listener(projected)
		}
	})
}

// Snapshot returns a read-only snapshot of the underlying agent state.
func (s *Session) Snapshot() agent.State { return s.agent.Snapshot() }

// Messages returns the current transcript.
func (s *Session) Messages() []agent.AgentMessage { return s.agent.Snapshot().Messages }

// Cwd returns the workspace root.
func (s *Session) Cwd() string { return s.cwd }

// buildSystemPrompt assembles the coding system prompt from the active tools'
// self-descriptions and the workspace's project context files.
func (s *Session) buildSystemPrompt(instructions string) string {
	infos := make([]prompt.ToolInfo, len(s.tools))
	for i, t := range s.tools {
		infos[i] = prompt.ToolInfo{
			Name:       t.Name(),
			Snippet:    t.PromptSnippet,
			Guidelines: t.Guidelines,
		}
	}
	return prompt.Build(prompt.Options{
		Instructions: instructions,
		Tools:        infos,
		ContextFiles: prompt.LoadContextFiles(s.cwd),
	})
}

// readOnlyNames maps each tool name to whether it is read-only, for the
// permission gate.
func readOnlyNames(toolSet []tools.Tool) map[string]bool {
	m := make(map[string]bool, len(toolSet))
	for _, t := range toolSet {
		m[t.Name()] = t.ReadOnly
	}
	return m
}
