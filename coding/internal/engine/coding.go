// Package engine owns one stateful coding-agent session. It wires the reusable
// agent and llm libraries to the product's tools, prompt, permissions, transcript,
// and compaction policy.
package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/prompt"
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
	// Skills are file-backed skills advertised to the model and loadable on
	// demand via the skill tool. Empty disables skills, and the tool is omitted.
	// Load them with skills.Load and pass the result here.
	Skills []skills.Skill
	// Policy classifies resolved tool access. Nil uses permission.DefaultPolicy.
	Policy permission.Policy
	// Approver obtains decisions for calls classified as Ask. Nil denies them.
	Approver permission.Approver
	// Store persists the transcript and seeds it on construction. Nil disables
	// persistence.
	Store transcript.Store
	// Compactor creates checkpoint summaries. Nil uses a native, tool-free LLM
	// request configured from StreamFn, StreamOptions, and GetAPIKey.
	Compactor compaction.Compactor
	// DetailsStore persists recognized structured tool results out of band, keyed
	// by tool-call ID, so a reloaded session restores rich rendering and preview
	// targets. Nil replays history as plain text.
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
	store      transcript.Store
	tools      []tools.Tool
	toolByName map[string]tools.Tool
	authorizer *permission.Service
	shells     *tools.BackgroundShells
	cwd        string
	skills     []skills.Skill

	maxRetries    int
	contextWindow int64
	compactor     compaction.Compactor

	runMu                sync.Mutex
	persistedLen         int
	runStateMu           sync.RWMutex
	runCtx               context.Context
	runStartedAt         time.Time
	runParentID          string
	autoCompactAttempted bool

	transcriptMu sync.RWMutex
	entries      []transcript.Entry
	leafID       string
	usageStart   int

	detailsStore transcript.DetailsStore
	detailsMu    sync.Mutex
	details      map[string]any // tool-call ID -> decoded structured Details

	eventMu        sync.Mutex
	eventListeners map[int]func(Event)
	nextEventID    int
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
	var shells *tools.BackgroundShells
	if toolSet == nil {
		toolSet, shells = tools.CodingToolsWithShells(cwd, tools.LocalOps{})
	}

	// A skill tool is added only when skills are configured. Loading already
	// discovered skill text is internal session access and needs no approval.
	if len(opts.Skills) > 0 {
		reg := skills.NewRegistry(opts.Skills)
		toolSet = append(toolSet, tools.Tool{AgentTool: reg.Tool(), AccessFor: tools.InternalAccess})
	}

	authorizer, err := permission.NewService(cwd, opts.Policy, opts.Approver)
	if err != nil {
		return nil, err
	}

	var entries []transcript.Entry
	if opts.Store != nil {
		loaded, err := opts.Store.Load(ctx)
		if err != nil {
			return nil, err
		}
		entries = loaded
	}
	leafID := ""
	if len(entries) > 0 {
		leafID = entries[len(entries)-1].ID
	}
	seed, err := transcript.BuildContext(entries, leafID)
	if err != nil {
		return nil, err
	}
	usageStart := 0
	path, err := transcript.BuildPath(entries, leafID)
	if err != nil {
		return nil, err
	}
	for _, entry := range path {
		if entry.Type == transcript.CompactionEntry {
			usageStart = len(seed)
		}
	}

	maxRetries := defaultMaxRetries
	if opts.MaxRetries != nil {
		maxRetries = *opts.MaxRetries
	}

	details := map[string]any{}
	if opts.DetailsStore != nil {
		stored, err := opts.DetailsStore.Load(ctx)
		if err != nil {
			return nil, err
		}
		for id, raw := range stored {
			if d := decodeDetails(raw); d != nil {
				details[id] = d
			}
		}
	}

	s := &Session{
		store:          opts.Store,
		tools:          toolSet,
		toolByName:     toolsByName(toolSet),
		authorizer:     authorizer,
		shells:         shells,
		cwd:            cwd,
		skills:         opts.Skills,
		maxRetries:     maxRetries,
		contextWindow:  opts.Model.ContextWindow,
		compactor:      opts.Compactor,
		persistedLen:   len(seed),
		entries:        append([]transcript.Entry(nil), entries...),
		leafID:         leafID,
		usageStart:     usageStart,
		detailsStore:   opts.DetailsStore,
		details:        details,
		eventListeners: make(map[int]func(Event)),
	}
	if s.compactor == nil {
		s.compactor = compaction.LLM{
			StreamFn: opts.StreamFn, StreamOptions: opts.StreamOptions,
			GetAPIKey: opts.GetAPIKey,
		}
	}

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
			var accesses []permission.Access
			if t, ok := s.toolByName[bc.ToolCall.Name]; ok {
				accesses = t.Accesses(args)
			}
			decision, _ := s.authorizer.Authorize(bc.RunContext, permission.Request{
				ToolCallID: bc.ToolCall.ID,
				Tool:       bc.ToolCall.Name,
				Args:       args,
				Accesses:   accesses,
			})
			return decision.Behavior != permission.Allow, decision.Reason
		},
		PrepareNextTurn: s.prepareNextTurn,
	}
	s.agent = agent.New(agentOpts)
	s.captureDetails()
	s.agent.Subscribe(func(ev agent.AgentEvent) {
		if projected, ok := projectAgentEvent(ev); ok {
			s.dispatchEvent(projected)
		}
	})

	return s, nil
}

// captureDetails subscribes to tool completions and retains each tool's
// structured Details in memory, persisting it to the DetailsStore so a later
// reload can restore it. It is registered once, for the session's lifetime.
func (s *Session) captureDetails() {
	s.agent.Subscribe(func(ev agent.AgentEvent) {
		if ev.Type != agent.ToolEnd {
			return
		}
		result, ok := ev.Result.(agent.ToolResult)
		if !ok || result.Details == nil {
			return
		}
		payload, ok := encodeDetails(result.Details)
		if !ok {
			return
		}
		s.detailsMu.Lock()
		s.details[ev.ToolCallID] = result.Details
		s.detailsMu.Unlock()
		if s.detailsStore != nil {
			// Persist out of band; a failure here must not disrupt the run, and the
			// live event already carried the Details to any subscriber.
			_ = s.detailsStore.Put(context.Background(), ev.ToolCallID, payload)
		}
	})
}

// snapshotDetails returns a copy of the captured tool-call details, safe to read
// while a run is appending more.
func (s *Session) snapshotDetails() map[string]any {
	s.detailsMu.Lock()
	defer s.detailsMu.Unlock()
	out := make(map[string]any, len(s.details))
	for id, d := range s.details {
		out[id] = d
	}
	return out
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
	// Flush any messages left in memory by an earlier store failure before this
	// run captures its durable parent. Otherwise their later persistence could be
	// mistaken for messages produced by the new run.
	if err := s.persistNew(ctx); err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	_, runParentID := s.snapshotTranscript()
	s.setRunState(ctx, startedAt, runParentID)
	defer s.clearRunState()
	s.dispatchEvent(Event{Type: RunStarted, StartedAt: startedAt})

	if s.shouldAutoCompact(s.ContextUsage().UsedTokens) {
		_, _ = s.autoCompact(ctx)
	}

	var runUsage llm.Usage
	unsubscribe := s.agent.Subscribe(func(event agent.AgentEvent) {
		if event.Type != agent.MessageEnd {
			return
		}
		if assistant, ok := eventAssistantMessage(event.Message); ok {
			addUsage(&runUsage, assistant.Usage)
		}
	})
	defer unsubscribe()

	runErr := fn(ctx)
	if runErr != nil && !s.trailingContextOverflow() && s.maxRetries > 0 {
		runErr = s.withRetry(ctx, runErr)
	}
	if s.trailingContextOverflow() {
		recovered, err := s.recoverContextOverflow(ctx, runErr)
		runErr = err
		if recovered && runErr != nil && s.maxRetries > 0 {
			runErr = s.withRetry(ctx, runErr)
		}
	}
	completedAt := time.Now().UTC()
	persistErr := s.persistNewRun(ctx, runParentID, startedAt, completedAt)
	s.dispatchEvent(Event{
		Type:        RunCompleted,
		Usage:       runUsage,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	})
	return errors.Join(runErr, persistErr)
}

// persistNew appends the messages added since the last persist to the Store. It
// runs only while runMu is held, so persistedLen is not racing a run.
func (s *Session) persistNew(ctx context.Context) error {
	return s.persistNewMessages(ctx, "", time.Time{}, time.Time{})
}

func (s *Session) persistNewRun(
	ctx context.Context,
	runParentID string,
	startedAt, completedAt time.Time,
) error {
	return s.persistNewMessages(ctx, runParentID, startedAt, completedAt)
}

func (s *Session) persistNewMessages(
	ctx context.Context,
	runParentID string,
	startedAt, completedAt time.Time,
) error {
	all := s.agent.Snapshot().Messages
	s.transcriptMu.RLock()
	persistedLen := s.persistedLen
	parentID := s.leafID
	existing := append([]transcript.Entry(nil), s.entries...)
	s.transcriptMu.RUnlock()
	var added []agent.AgentMessage
	if persistedLen < len(all) {
		added = all[persistedLen:]
	}
	entries := make([]transcript.Entry, 0, len(added)+1)
	for _, message := range added {
		entry := transcript.NewMessage(parentID, message)
		entries = append(entries, entry)
		parentID = entry.ID
	}
	if !startedAt.IsZero() && !completedAt.IsZero() {
		candidate := append(existing, entries...)
		firstEntryID, err := firstMessageAfter(candidate, parentID, runParentID)
		if err != nil {
			return err
		}
		runEntry := transcript.NewRun(parentID, firstEntryID, startedAt, completedAt)
		entries = append(entries, runEntry)
		parentID = runEntry.ID
	}
	if len(entries) == 0 {
		return nil
	}
	if s.store != nil {
		if err := s.store.Append(ctx, entries...); err != nil {
			return err
		}
	}
	s.transcriptMu.Lock()
	s.entries = append(s.entries, entries...)
	s.leafID = parentID
	s.persistedLen = len(all)
	s.transcriptMu.Unlock()
	return nil
}

func firstMessageAfter(
	entries []transcript.Entry,
	leafID string,
	ancestorID string,
) (string, error) {
	path, err := transcript.BuildPath(entries, leafID)
	if err != nil {
		return "", err
	}
	afterAncestor := ancestorID == ""
	foundAncestor := afterAncestor
	for _, entry := range path {
		if !afterAncestor {
			if entry.ID == ancestorID {
				afterAncestor = true
				foundAncestor = true
			}
			continue
		}
		if entry.Type == transcript.MessageEntry {
			return entry.ID, nil
		}
	}
	if !foundAncestor {
		return "", errors.New("coding: run parent is not on the active transcript path")
	}
	return "", nil
}

func (s *Session) setRunState(ctx context.Context, startedAt time.Time, parentID string) {
	s.runStateMu.Lock()
	s.runCtx = ctx
	s.runStartedAt = startedAt
	s.runParentID = parentID
	s.autoCompactAttempted = false
	s.runStateMu.Unlock()
}

func (s *Session) clearRunState() {
	s.runStateMu.Lock()
	s.runCtx = nil
	s.runStartedAt = time.Time{}
	s.runParentID = ""
	s.autoCompactAttempted = false
	s.runStateMu.Unlock()
}

func (s *Session) activeRunState() (context.Context, time.Time, string) {
	s.runStateMu.RLock()
	defer s.runStateMu.RUnlock()
	return s.runCtx, s.runStartedAt, s.runParentID
}

// QueueHandle identifies one message while it remains queued in this Session.
type QueueHandle struct {
	agent agent.QueueHandle
}

// Steer queues a message to inject after the current turn's tool calls finish.
func (s *Session) Steer(text string, images ...llm.ImageContent) QueueHandle {
	return QueueHandle{agent: s.agent.Steer(agent.UserMessage(text, images...))}
}

// FollowUp queues a message to process once the run would otherwise stop.
func (s *Session) FollowUp(text string, images ...llm.ImageContent) QueueHandle {
	return QueueHandle{agent: s.agent.FollowUp(agent.UserMessage(text, images...))}
}

// CancelQueuedMessage removes one message that has not entered the transcript.
func (s *Session) CancelQueuedMessage(handle QueueHandle) bool {
	return s.agent.CancelQueued(handle.agent)
}

// Abort cancels an in-progress run.
func (s *Session) Abort() { s.agent.Abort() }

// Close releases resources the session owns. It stops any background shells the
// default tool set started so long-lived processes do not outlive the session.
// It does not abort an in-progress run; call Abort first if one may be active.
// Close is safe to call more than once, and a no-op when the session was built
// with a caller-supplied tool set.
func (s *Session) Close() {
	if s.shells != nil {
		s.shells.Shutdown()
	}
}

// ClearQueuedMessages drops steering and follow-up messages that have not yet
// entered the transcript. Product adapters use it when a run is stopped or
// otherwise finishes before a queued message can be consumed.
func (s *Session) ClearQueuedMessages() { s.agent.ClearQueues() }

// Subscribe registers a listener for UI-neutral coding events and returns a
// function that removes it.
func (s *Session) Subscribe(listener func(Event)) (unsubscribe func()) {
	s.eventMu.Lock()
	id := s.nextEventID
	s.nextEventID++
	s.eventListeners[id] = listener
	s.eventMu.Unlock()
	return func() {
		s.eventMu.Lock()
		delete(s.eventListeners, id)
		s.eventMu.Unlock()
	}
}

func (s *Session) dispatchEvent(event Event) {
	s.eventMu.Lock()
	listeners := make([]func(Event), 0, len(s.eventListeners))
	for _, listener := range s.eventListeners {
		listeners = append(listeners, listener)
	}
	s.eventMu.Unlock()
	for _, listener := range listeners {
		listener(event)
	}
}

// Snapshot returns a read-only snapshot of the underlying agent state.
func (s *Session) Snapshot() agent.State { return s.agent.Snapshot() }

// Messages returns every original message on the current transcript path. A
// compacted session therefore still exposes its complete history.
func (s *Session) Messages() []agent.AgentMessage {
	s.transcriptMu.RLock()
	entries := append([]transcript.Entry(nil), s.entries...)
	leafID := s.leafID
	persistedLen := s.persistedLen
	s.transcriptMu.RUnlock()
	messages, err := transcript.Messages(entries, leafID)
	if err != nil {
		return nil
	}
	active := s.agent.Snapshot().Messages
	if persistedLen < len(active) {
		messages = append(messages, active[persistedLen:]...)
	}
	return messages
}

// Entries returns a detached snapshot of the durable session log.
func (s *Session) Entries() []transcript.Entry {
	entries, _ := s.snapshotTranscript()
	return entries
}

// Cwd returns the workspace root.
func (s *Session) Cwd() string { return s.cwd }

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

func (s *Session) snapshotTranscript() ([]transcript.Entry, string) {
	entries, leafID, _ := s.snapshotTranscriptState()
	return entries, leafID
}

func (s *Session) snapshotTranscriptState() ([]transcript.Entry, string, int) {
	s.transcriptMu.RLock()
	defer s.transcriptMu.RUnlock()
	return append([]transcript.Entry(nil), s.entries...), s.leafID, s.persistedLen
}

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
		Instructions:  instructions,
		WorkspaceRoot: s.cwd,
		Tools:         infos,
		ContextFiles:  prompt.LoadContextFiles(s.cwd),
		Skills:        skillInfos(s.skills),
	})
}

// skillInfos projects loaded skills into the prompt's listing entries.
func skillInfos(skills []skills.Skill) []prompt.SkillInfo {
	if len(skills) == 0 {
		return nil
	}
	infos := make([]prompt.SkillInfo, len(skills))
	for i, sk := range skills {
		infos[i] = prompt.SkillInfo{Name: sk.Name, Description: sk.Description}
	}
	return infos
}

// toolsByName indexes the tool set by advertised name for access description.
func toolsByName(toolSet []tools.Tool) map[string]tools.Tool {
	m := make(map[string]tools.Tool, len(toolSet))
	for _, t := range toolSet {
		m[t.Name()] = t
	}
	return m
}
