// Package engine owns one stateful coding-agent session. It wires the reusable
// agent and llm libraries to the product's tools, prompt, permissions, transcript,
// and compaction policy.
package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/compaction"
	"github.com/ktsoator/or/coding/internal/modelcontext"
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
	// Skills is the initial immutable skill snapshot. The stable skill tool is
	// present even when this slice is empty.
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
	// Browser delivers open_preview commands to the product shell and waits for
	// a terminal navigation acknowledgement. Nil makes the tool fail closed.
	Browser tools.BrowserController
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
	agent                *agent.Agent
	store                transcript.Store
	tools                []tools.Tool
	toolByName           map[string]tools.Tool
	authorizer           *permission.Service
	shells               *tools.BackgroundShells
	cwd                  string
	skillRegistry        *skills.DynamicRegistry
	skillLoader          func() []skills.Skill
	skillRevision        string
	pendingSkills        *skills.Registry
	pendingSkillRevision string
	modelContext         *modelcontext.Manager

	maxRetries    int
	contextWindow int64
	compactor     compaction.Compactor

	runMu                sync.Mutex
	persistedLen         int
	runStateMu           sync.RWMutex
	runCtx               context.Context
	runStartedAt         time.Time
	runEntryStart        int
	autoCompactAttempted bool
	runPersistenceErr    error

	transcriptMu sync.RWMutex
	entries      []transcript.Entry
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
		toolSet, shells = tools.CodingToolsWithShells(cwd, tools.LocalOps{}, opts.Browser)
	}

	initialSkills := opts.Skills
	if opts.SkillLoader != nil {
		initialSkills = opts.SkillLoader()
	}
	initialRegistry := skills.NewRegistry(initialSkills)
	dynamicSkills := skills.NewDynamicRegistry(initialRegistry)
	// The tool schema and closure have session lifetime even when no skill is
	// currently installed. Only the immutable registry behind the closure moves.
	toolSet = upsertTool(toolSet, tools.Tool{
		AgentTool: dynamicSkills.Tool(),
		AccessFor: tools.InternalAccess,
	})

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
	seed, err := transcript.BuildContext(entries)
	if err != nil {
		return nil, err
	}
	usageStart := 0
	for _, entry := range entries {
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
		skillRegistry:  dynamicSkills,
		skillLoader:    opts.SkillLoader,
		skillRevision:  initialRegistry.Revision(),
		maxRetries:     maxRetries,
		contextWindow:  opts.Model.ContextWindow,
		compactor:      opts.Compactor,
		persistedLen:   len(seed),
		entries:        append([]transcript.Entry(nil), entries...),
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
	s.modelContext = modelcontext.New(
		nextContextEpoch(entries),
		s.buildBaseContext(),
		s.skillRevision,
		s.buildSkillListing(),
	)

	agentOpts := agent.Options{
		SystemPrompt:  s.buildSystemPrompt(opts.Instructions),
		Model:         opts.Model,
		ThinkingLevel: opts.ThinkingLevel,
		Tools:         tools.AgentTools(toolSet),
		Messages:      seed,
		StreamOptions: opts.StreamOptions,
		StreamFn:      s.modelStreamFn(opts.StreamFn),
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

// run serializes a single Prompt or Continue invocation. Model-request prefixes
// are checkpointed during the run, and the final assistant plus run metadata
// are flushed when it completes.
func (s *Session) run(ctx context.Context, fn func(context.Context) error) error {
	if !s.runMu.TryLock() {
		return ErrBusy
	}
	defer s.runMu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	// Flush any messages left in memory by an earlier store failure before this
	// run captures its durable starting position. Otherwise their later
	// persistence could be mistaken for messages produced by the new run.
	if err := s.persistNew(ctx); err != nil {
		return err
	}
	s.prepareSkillRefresh()
	startedAt := time.Now().UTC()
	runEntryStart := len(s.snapshotTranscript())
	s.setRunState(ctx, startedAt, runEntryStart)
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
	checkpointErr := s.runPersistenceError()
	if checkpointErr == nil && runErr != nil && !s.trailingContextOverflow() && s.maxRetries > 0 {
		runErr = s.withRetry(ctx, runErr)
		checkpointErr = s.runPersistenceError()
	}
	if checkpointErr == nil && s.trailingContextOverflow() {
		recovered, err := s.recoverContextOverflow(ctx, runErr)
		runErr = err
		checkpointErr = s.runPersistenceError()
		if checkpointErr == nil && recovered && runErr != nil && s.maxRetries > 0 {
			runErr = s.withRetry(ctx, runErr)
			checkpointErr = s.runPersistenceError()
		}
	}
	if checkpointErr != nil {
		// A StreamFn setup failure becomes a synthetic assistant error inside the
		// reusable agent. This error belongs to the persistence layer, not the
		// conversation, so remove it before the final flush and never feed it into
		// model retry or context-overflow recovery.
		s.dropTrailingErrorTurn()
		runErr = checkpointErr
	}
	completedAt := time.Now().UTC()
	persistErr := s.persistNewRun(ctx, runEntryStart, startedAt, completedAt)
	s.dispatchEvent(Event{
		Type:        RunCompleted,
		Usage:       runUsage,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	})
	return errors.Join(runErr, persistErr)
}

// modelStreamFn is the product's model-request boundary. It projects hidden
// product context into a detached provider input, checkpoints the canonical
// messages and any newly emitted context attachments, then reaches the
// provider. A persistence failure prevents both context commit and provider I/O.
func (s *Session) modelStreamFn(delegate agent.StreamFn) agent.StreamFn {
	if delegate == nil {
		delegate = llm.Stream
	}
	return func(
		ctx context.Context,
		model llm.Model,
		input llm.Context,
		options llm.StreamOptions,
	) (<-chan llm.Event, error) {
		prepared := s.modelContext.PrepareStep(input)
		if err := s.persistModelInput(ctx, input.Messages, prepared.Pending); err != nil {
			checkpointErr := fmt.Errorf("coding: persist model request checkpoint: %w", err)
			s.recordRunPersistenceError(checkpointErr)
			return nil, checkpointErr
		}
		s.modelContext.Commit(prepared)
		s.commitSkillRefresh(prepared.Pending)
		return delegate(ctx, model, prepared.Input, options)
	}
}

// persistModelInput appends the canonical request prefix and any new hidden
// attachments before the provider is called. input is authoritative here:
// RunLoop may emit MessageEnd before Agent has reduced that event into its live
// snapshot, while input already contains the complete canonical prefix.
func (s *Session) persistModelInput(
	ctx context.Context,
	input []llm.Message,
	attachments []modelcontext.Attachment,
) error {
	messages := make([]agent.AgentMessage, len(input))
	for index, message := range input {
		messages[index] = agent.FromLLM(message)
	}
	contextEntries := make([]transcript.Entry, len(attachments))
	for index, attachment := range attachments {
		contextEntries[index] = transcript.NewContext(transcript.ContextAttachment{
			AttachmentID: attachment.ID,
			Epoch:        attachment.Epoch,
			Kind:         string(attachment.Kind),
			Placement:    string(attachment.Placement),
			Path:         attachment.Path,
			Revision:     attachment.Revision,
			Rendered:     attachment.Rendered,
		})
	}
	return s.persistMessages(
		ctx,
		messages,
		contextEntries,
		0,
		time.Time{},
		time.Time{},
	)
}

// persistNew appends the messages added since the last persist to the Store. It
// runs only while runMu is held, so persistedLen is not racing a run.
func (s *Session) persistNew(ctx context.Context) error {
	return s.persistNewMessages(ctx, 0, time.Time{}, time.Time{})
}

func (s *Session) persistNewRun(
	ctx context.Context,
	runEntryStart int,
	startedAt, completedAt time.Time,
) error {
	return s.persistNewMessages(ctx, runEntryStart, startedAt, completedAt)
}

func (s *Session) persistNewMessages(
	ctx context.Context,
	runEntryStart int,
	startedAt, completedAt time.Time,
) error {
	return s.persistMessages(
		ctx,
		s.agent.Snapshot().Messages,
		nil,
		runEntryStart,
		startedAt,
		completedAt,
	)
}

func (s *Session) persistMessages(
	ctx context.Context,
	all []agent.AgentMessage,
	contextEntries []transcript.Entry,
	runEntryStart int,
	startedAt, completedAt time.Time,
) error {
	s.transcriptMu.RLock()
	persistedLen := s.persistedLen
	existing := append([]transcript.Entry(nil), s.entries...)
	s.transcriptMu.RUnlock()
	if persistedLen > len(all) {
		return fmt.Errorf(
			"coding: cannot persist context with %d messages behind durable prefix of %d",
			len(all),
			persistedLen,
		)
	}
	var added []agent.AgentMessage
	if persistedLen < len(all) {
		added = all[persistedLen:]
	}
	entries := make([]transcript.Entry, 0, len(contextEntries)+len(added)+1)
	entries = append(entries, contextEntries...)
	for _, message := range added {
		entries = append(entries, transcript.NewMessage(message))
	}
	if !startedAt.IsZero() && !completedAt.IsZero() {
		candidate := append(existing, entries...)
		firstEntryID := firstMessageFrom(candidate, runEntryStart)
		entries = append(entries, transcript.NewRun(firstEntryID, startedAt, completedAt))
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
	s.persistedLen = len(all)
	s.transcriptMu.Unlock()
	return nil
}

func firstMessageFrom(entries []transcript.Entry, start int) string {
	if start < 0 || start >= len(entries) {
		return ""
	}
	for _, entry := range entries[start:] {
		if entry.Type == transcript.MessageEntry {
			return entry.ID
		}
	}
	return ""
}

func (s *Session) setRunState(ctx context.Context, startedAt time.Time, entryStart int) {
	s.runStateMu.Lock()
	s.runCtx = ctx
	s.runStartedAt = startedAt
	s.runEntryStart = entryStart
	s.autoCompactAttempted = false
	s.runPersistenceErr = nil
	s.runStateMu.Unlock()
}

func (s *Session) clearRunState() {
	s.runStateMu.Lock()
	s.runCtx = nil
	s.runStartedAt = time.Time{}
	s.runEntryStart = 0
	s.autoCompactAttempted = false
	s.runPersistenceErr = nil
	s.runStateMu.Unlock()
}

func (s *Session) recordRunPersistenceError(err error) {
	if err == nil {
		return
	}
	s.runStateMu.Lock()
	if s.runPersistenceErr == nil {
		s.runPersistenceErr = err
	}
	s.runStateMu.Unlock()
}

func (s *Session) runPersistenceError() error {
	s.runStateMu.RLock()
	defer s.runStateMu.RUnlock()
	return s.runPersistenceErr
}

func (s *Session) activeRunState() (context.Context, time.Time, int) {
	s.runStateMu.RLock()
	defer s.runStateMu.RUnlock()
	return s.runCtx, s.runStartedAt, s.runEntryStart
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
	persistedLen := s.persistedLen
	s.transcriptMu.RUnlock()
	messages := transcript.Messages(entries)
	active := s.agent.Snapshot().Messages
	if persistedLen < len(active) {
		messages = append(messages, active[persistedLen:]...)
	}
	return messages
}

// Entries returns a detached snapshot of the durable session log.
func (s *Session) Entries() []transcript.Entry {
	return s.snapshotTranscript()
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

func (s *Session) snapshotTranscript() []transcript.Entry {
	entries, _ := s.snapshotTranscriptState()
	return entries
}

func (s *Session) snapshotTranscriptState() ([]transcript.Entry, int) {
	s.transcriptMu.RLock()
	defer s.transcriptMu.RUnlock()
	return append([]transcript.Entry(nil), s.entries...), s.persistedLen
}

// buildSystemPrompt assembles only the stable coding prompt. Project
// instructions and skill listings are projected by modelContext at the request
// boundary and never become part of the Agent's canonical system prompt.
func (s *Session) buildSystemPrompt(instructions string) string {
	infos := make([]prompt.ToolInfo, len(s.tools))
	for i, t := range s.tools {
		infos[i] = prompt.ToolInfo{
			Name:       t.Name(),
			Snippet:    t.PromptSnippet,
			Guidelines: t.Guidelines,
		}
	}
	return prompt.BuildSystem(prompt.SystemOptions{
		Instructions:  instructions,
		WorkspaceRoot: s.cwd,
		Tools:         infos,
	})
}

func (s *Session) buildBaseContext() string {
	return prompt.RenderBaseContext(prompt.LoadContextFiles(s.cwd))
}

func (s *Session) buildSkillListing() string {
	return prompt.RenderSkillListing(
		s.skillRevision,
		skillInfos(s.skillRegistry.List()),
	)
}

// prepareSkillRefresh stages one immutable snapshot for the next request. The
// live tool registry is not replaced here; modelStreamFn publishes it only after
// the matching hidden update and canonical request prefix are durable.
func (s *Session) prepareSkillRefresh() {
	if s.skillLoader == nil {
		return
	}
	next := skills.NewRegistry(s.skillLoader())
	nextRevision := next.Revision()

	if s.pendingSkills != nil {
		switch nextRevision {
		case s.pendingSkillRevision:
			return
		case s.skillRevision:
			s.pendingSkills = nil
			s.pendingSkillRevision = ""
			s.modelContext.CancelStagedSkillsUpdate()
			return
		}
	} else if nextRevision == s.skillRevision {
		return
	}

	delta := skills.Diff(s.skillRegistry.Snapshot(), next)
	rendered := prompt.RenderSkillsUpdate(
		nextRevision,
		skillInfos(next.List()),
		promptSkillDelta(delta),
	)
	s.pendingSkills = next
	s.pendingSkillRevision = nextRevision
	s.modelContext.StageSkillsUpdate(nextRevision, rendered)
}

func (s *Session) commitSkillRefresh(attachments []modelcontext.Attachment) {
	if s.pendingSkills == nil {
		return
	}
	for _, attachment := range attachments {
		if attachment.Kind != modelcontext.SkillsUpdate ||
			attachment.Revision != s.pendingSkillRevision {
			continue
		}
		s.skillRegistry.Replace(s.pendingSkills)
		s.skillRevision = s.pendingSkillRevision
		s.pendingSkills = nil
		s.pendingSkillRevision = ""
		return
	}
}

func nextContextEpoch(entries []transcript.Entry) uint64 {
	var latest uint64
	for _, entry := range entries {
		if entry.Type == transcript.ContextEntry &&
			entry.Context != nil &&
			entry.Context.Epoch > latest {
			latest = entry.Context.Epoch
		}
	}
	return latest + 1
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

func promptSkillDelta(delta skills.Delta) prompt.SkillsDelta {
	return prompt.SkillsDelta{
		Added:   skillInfos(delta.Added),
		Updated: skillInfos(delta.Updated),
		Removed: append([]string(nil), delta.Removed...),
	}
}

func upsertTool(toolSet []tools.Tool, builtIn tools.Tool) []tools.Tool {
	result := append([]tools.Tool(nil), toolSet...)
	for index := range result {
		if result[index].Name() == builtIn.Name() {
			result[index] = builtIn
			return result
		}
	}
	return append(result, builtIn)
}

// toolsByName indexes the tool set by advertised name for access description.
func toolsByName(toolSet []tools.Tool) map[string]tools.Tool {
	m := make(map[string]tools.Tool, len(toolSet))
	for _, t := range toolSet {
		m[t.Name()] = t
	}
	return m
}
