// Package engine owns one stateful coding-agent session. It wires the reusable
// agent and llm libraries to the product's tools, prompt, permissions, transcript,
// and compaction policy.
package engine

import (
	"errors"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
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
	// Skills is the initial immutable skill snapshot. The Skill tool is advertised
	// only while the active snapshot contains at least one Skill.
	Skills []skills.Skill
	// SkillLoader refreshes the resolved skill snapshot once at session
	// construction and once before every top-level Prompt or Continue. Nil keeps
	// Skills static. The loader is deliberately not called for provider retries,
	// tool-loop turns, or context-overflow recovery.
	SkillLoader func() []skills.Skill
	// Policy classifies resolved tool access. Nil uses permission.DefaultPolicy.
	Policy permission.Policy
	// Approver obtains decisions for calls classified as Ask. Nil denies them.
	Approver permission.Approver
	// Browser delivers navigation and read-only observation requests to the
	// product shell and waits for their acknowledgements. Nil makes those tools
	// fail closed.
	Browser tools.BrowserController
	// Asker puts a multiple-choice question to the user and blocks until they
	// answer. Nil advertises no question tool at all, so a session with nobody
	// at the keyboard never sees one it cannot use.
	Asker tools.Asker
	// Store persists the transcript and seeds it on construction. Nil disables
	// persistence.
	Store transcript.Store
	// Compactor creates checkpoint summaries. Nil uses a native, tool-free LLM
	// request configured from StreamFn, StreamOptions, and GetAPIKey.
	Compactor compaction.Compactor
	// DetailsStore persists machine-readable tool outcomes out of band, keyed by
	// tool-call ID, so a reloaded session restores status, error metadata, rich
	// rendering, and preview targets. Nil replays without structured data.
	DetailsStore transcript.DetailsStore
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
	agent      *agent.Agent
	journal    *sessionJournal
	tools      []tools.Tool
	allTools   []tools.Tool
	toolByName map[string]tools.Tool
	authorizer *permission.Service
	tasks      *tools.TaskManager

	taskUnsubscribe func()
	cwd             string

	skillRegistry        *skills.DynamicRegistry
	skillLoader          func() []skills.Skill
	skillRevision        string
	pendingSkills        *skills.Registry
	pendingSkillRevision string

	contextRevision        string
	pendingContextRevision string
	modelContext           *modelcontext.Manager
	instructions           string

	maxRetries    int
	contextWindow int64
	compactor     compaction.Compactor

	runMu    sync.Mutex
	runState sessionRunState
	events   eventBus
}

// QueueHandle identifies one message submitted to this Session's queue. The
// identity remains stable when the message enters the run.
type QueueHandle struct {
	agent agent.QueueHandle
}

// Steer queues a message to inject after the current turn's tool calls finish.
func (s *Session) Steer(text string, images ...llm.ImageContent) QueueHandle {
	return QueueHandle{agent: s.agent.Steer(agent.UserMessage(text, images...))}
}

// SteerWithFiles queues guidance with product-owned attached file context.
func (s *Session) SteerWithFiles(
	text string,
	files []AttachedFile,
	images ...llm.ImageContent,
) QueueHandle {
	return QueueHandle{agent: s.agent.Steer(userMessage(text, files, images))}
}

// FollowUp queues a message to process once the run would otherwise stop.
func (s *Session) FollowUp(text string, images ...llm.ImageContent) QueueHandle {
	return QueueHandle{agent: s.agent.FollowUp(agent.UserMessage(text, images...))}
}

// FollowUpWithFiles queues a follow-up with product-owned attached file context.
func (s *Session) FollowUpWithFiles(
	text string,
	files []AttachedFile,
	images ...llm.ImageContent,
) QueueHandle {
	return QueueHandle{agent: s.agent.FollowUp(userMessage(text, files, images))}
}

// CancelQueuedMessage removes one message that has not entered the transcript.
func (s *Session) CancelQueuedMessage(handle QueueHandle) bool {
	return s.agent.CancelQueued(handle.agent)
}

// Abort cancels an in-progress run.
func (s *Session) Abort() { s.agent.Abort() }

// Close releases resources the session owns. It stops any background tasks the
// default tool set started so long-lived processes do not outlive the session.
// It does not abort an in-progress run; call Abort first if one may be active.
// Close is safe to call more than once, and a no-op when the session was built
// with a caller-supplied tool set.
func (s *Session) Close() {
	if s.taskUnsubscribe != nil {
		s.taskUnsubscribe()
	}
	if s.tasks != nil {
		s.tasks.Shutdown()
	}
}

// ClearQueuedMessages drops steering and follow-up messages that have not yet
// entered the transcript. Product adapters use it when a run is stopped or
// otherwise finishes before a queued message can be consumed.
func (s *Session) ClearQueuedMessages() { s.agent.ClearQueues() }

// Snapshot returns a read-only snapshot of the underlying agent state.
func (s *Session) Snapshot() agent.State { return s.agent.Snapshot() }

// SetModel replaces the model used by the next run. Call it only while the
// session is idle; an in-flight run has already captured its model.
func (s *Session) SetModel(model llm.Model) {
	s.agent.SetModel(model)
	s.contextWindow = model.ContextWindow
}

// SetThinkingLevel replaces the reasoning effort used by the next run. Call it
// only while the session is idle.
func (s *Session) SetThinkingLevel(level llm.ModelThinkingLevel) {
	s.agent.SetThinkingLevel(level)
}

// SetPermissionPolicy replaces the authorization policy used by subsequent
// tool calls. Call it only while the session is idle.
func (s *Session) SetPermissionPolicy(policy permission.Policy) {
	s.authorizer.SetPolicy(policy)
}
