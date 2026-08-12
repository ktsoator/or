package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/mcp"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Manager owns every conversation across the registered workspaces. Metadata
// is kept in indexes while each transcript stays separate.
// Lock ordering: mu is always taken before the workspace registry's own lock.
// The registry never calls back into this package, so that ordering holds
// simply by never taking mu inside a registry call.
type Manager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	indexPath  string
	scratch    *workspace.Scratch
	workspaces *workspace.Registry
	// newTransport builds each session's link to its viewers. The delivery
	// layer supplies it, so this package never names a transport type.
	newTransport  NewTransport
	generateTitle titleGenerator
	streamFn      agent.StreamFn
	mcp           *mcp.Manager

	mu        sync.RWMutex
	sessions  map[string]*sessionRuntime
	usage     *usage.Store
	closed    bool
	tasks     sync.WaitGroup
	closeOnce sync.Once
}

// Options supplies the product services and storage root owned by a Manager.
type Options struct {
	DataDir      string
	Usage        *usage.Store
	Workspaces   *workspace.Registry
	NewTransport NewTransport
	MCP          *mcp.Manager
	// StreamFn overrides model streaming for every managed session. Production
	// leaves it nil; tests and embedded adapters can supply a deterministic model.
	StreamFn agent.StreamFn
}

// NewManager restores and validates the session index. Restored transcripts,
// transports, and engine sessions stay unloaded until the conversation is
// first opened. The ledger and registry are passed in because the HTTP layer
// also serves them directly.
func NewManager(ctx context.Context, opts Options) (*Manager, error) {
	ctx, cancel := context.WithCancel(ctx)
	dir := filepath.Join(opts.DataDir, "sessions")
	m := &Manager{
		ctx:           ctx,
		cancel:        cancel,
		indexPath:     filepath.Join(dir, "index.json"),
		scratch:       workspace.NewScratch(opts.DataDir),
		workspaces:    opts.Workspaces,
		newTransport:  opts.NewTransport,
		generateTitle: generateAITitle,
		streamFn:      opts.StreamFn,
		mcp:           opts.MCP,
		sessions:      make(map[string]*sessionRuntime),
		usage:         opts.Usage,
	}
	if err := transcript.SecurePrivatePermissions(dir); err != nil {
		cancel()
		return nil, fmt.Errorf("session: secure transcript storage: %w", err)
	}

	records, err := m.loadRecords()
	if err != nil {
		cancel()
		return nil, err
	}
	for _, record := range records {
		record, err = m.normalizeRecord(record)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("session: restore session %s: %w", record.ID, err)
		}
		m.sessions[record.ID] = newSessionRuntime(record)
	}
	if err := m.saveLocked(); err != nil {
		cancel()
		return nil, err
	}
	return m, nil
}

// Close stops accepting new work, cancels active runs and title generation,
// then releases every session-owned process. It is safe to call repeatedly.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		runtimes := make([]*sessionRuntime, 0, len(m.sessions))
		for _, runtime := range m.sessions {
			runtimes = append(runtimes, runtime)
		}
		m.mu.Unlock()

		m.cancel()
		for _, runtime := range runtimes {
			runtime.stop()
		}
		m.tasks.Wait()
		for _, runtime := range runtimes {
			runtime.close()
		}
	})
}

// EnsureLoaded opens one restored conversation on first use. Loading is
// serialized by the manager lock so concurrent history and SSE requests share
// one engine session and transport.
func (m *Manager) EnsureLoaded(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagerClosed
	}
	_, err := m.loadRuntimeLocked(id)
	return err
}

// ReleaseIfIdle unloads an engine-backed runtime once its delivery layer can
// atomically prove that no viewer is attached. The durable record remains in
// the manager so a later history or SSE request restores the session lazily.
// Active runs, viewer decisions, title generation, and live background tasks
// pin the runtime because closing any of them would change user-visible work.
func (m *Manager) ReleaseIfIdle(id string) bool {
	m.mu.Lock()
	runtime, ok := m.sessions[id]
	if m.closed || !ok || runtime.session == nil || runtime.transport == nil ||
		runtime.running.Load() || runtime.live.Load() || runtime.awaitingUser() ||
		runtime.titleGenerating.Load() || runtime.hasRunningTask() {
		m.mu.Unlock()
		return false
	}
	closer, ok := runtime.transport.(idleClosingTransport)
	if !ok || !closer.TryCloseIfIdle() {
		m.mu.Unlock()
		return false
	}

	unloaded := newSessionRuntime(runtime.record)
	m.sessions[id] = unloaded
	m.mu.Unlock()

	runtime.close()
	return true
}

func (m *Manager) loadRuntimeLocked(id string) (*sessionRuntime, error) {
	runtime, ok := m.sessions[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	if runtime.session != nil {
		return runtime, nil
	}
	if m.closed {
		return nil, ErrManagerClosed
	}

	loaded, err := m.build(runtime.record)
	if err != nil {
		return nil, fmt.Errorf("session: load session %s: %w", id, err)
	}
	entries := loaded.session.Entries()
	if loaded.record.UsageBackfillOffset < 0 || loaded.record.UsageBackfillOffset > len(entries) {
		loaded.close()
		return nil, fmt.Errorf(
			"session: invalid usage backfill offset %d for session %s with %d entries",
			loaded.record.UsageBackfillOffset,
			id,
			len(entries),
		)
	}
	if err := m.usage.BackfillEntries(id, entries[loaded.record.UsageBackfillOffset:]); err != nil {
		loaded.close()
		return nil, fmt.Errorf("session: backfill usage for session %s: %w", id, err)
	}
	m.sessions[id] = loaded
	if loaded.record != runtime.record {
		if err := m.saveLocked(); err != nil {
			m.sessions[id] = runtime
			loaded.close()
			return nil, err
		}
	}
	return loaded, nil
}

func newSessionRuntime(record record) *sessionRuntime {
	return &sessionRuntime{record: record}
}

func (m *Manager) normalizeRecord(record record) (record, error) {
	if record.Scope != ScopeChat && record.Scope != ScopeProject {
		return record, fmt.Errorf("session: invalid session scope %q", record.Scope)
	}
	if record.WorkspaceKind != KindScratch && record.WorkspaceKind != KindFolder {
		return record, fmt.Errorf("session: invalid workspace kind %q", record.WorkspaceKind)
	}
	if record.Scope == ScopeChat && record.WorkspaceKind != KindScratch {
		return record, fmt.Errorf("session: chat session requires a scratch workspace")
	}
	if record.Scope == ScopeProject && record.WorkspaceKind != KindFolder {
		return record, fmt.Errorf("session: project session requires a folder workspace")
	}
	workspacePath, err := workspace.Clean(record.WorkspacePath)
	if err != nil {
		return record, err
	}
	if record.WorkspaceKind == KindScratch {
		workspacePath, err = m.scratch.Validate(workspacePath)
		if err != nil {
			return record, err
		}
	}
	if record.ForkedFromSessionID == record.ID {
		return record, errors.New("session: fork source cannot be the session itself")
	}
	if record.ForkedFromSessionID == "" && record.ForkedFromMessageID != "" {
		return record, errors.New("session: fork source metadata is incomplete")
	}
	if record.UsageBackfillOffset < 0 {
		return record, errors.New("session: usage backfill offset cannot be negative")
	}
	record.WorkspacePath = workspacePath
	model, ok := llm.LookupModel(record.Provider, record.Model)
	if !ok {
		return record, fmt.Errorf("unknown model %q for provider %q", record.Model, record.Provider)
	}
	thinking := llm.ClampThinkingLevel(model, llm.ModelThinkingLevel(record.Thinking))
	record.Provider = model.Provider
	record.Model = model.ID
	record.Thinking = string(thinking)
	permissionMode := permission.NormalizeMode(permission.Mode(record.PermissionMode))
	record.PermissionMode = string(permissionMode)
	return record, nil
}

func (m *Manager) build(record record) (*sessionRuntime, error) {
	record, err := m.normalizeRecord(record)
	if err != nil {
		return nil, err
	}
	if record.WorkspaceKind == KindScratch {
		if err := workspace.EnsureDirectories(record.WorkspacePath); err != nil {
			return nil, err
		}
	}
	model, _ := llm.LookupModel(record.Provider, record.Model)
	thinking := llm.ModelThinkingLevel(record.Thinking)
	permissionMode := permission.Mode(record.PermissionMode)
	transport := m.newTransport(record.ID)
	var mcpLease *mcp.Lease
	var additionalTools []tools.Tool
	if m.mcp != nil {
		mcpLease = m.mcp.Acquire(m.ctx, record.WorkspacePath)
		additionalTools = mcpLease.Tools()
	}
	session, err := newEngineSession(m.ctx, engineSessionConfig{
		WorkspacePath:   record.WorkspacePath,
		TranscriptPath:  record.Transcript,
		Model:           model,
		ThinkingLevel:   thinking,
		PermissionMode:  permissionMode,
		AdditionalTools: additionalTools,
		StreamFn:        m.streamFn,
	}, transport)
	if err != nil {
		mcpLease.Close()
		transport.Close()
		return nil, err
	}
	runtime := newSessionRuntime(record)
	runtime.session = session
	runtime.transport = transport
	runtime.mcpLease = mcpLease
	session.Subscribe(func(ev engine.Event) {
		m.handleSessionEvent(record.ID, runtime, ev)
	})
	if record.GenerateTitle {
		for _, item := range session.History() {
			if item.Type == engine.HistoryUser && strings.TrimSpace(item.Text) != "" {
				runtime.record.Title = titleFromPrompt(item.Text)
				break
			}
		}
	}
	return runtime, nil
}

func (s *sessionRuntime) stop() {
	s.running.Store(false)
	s.live.Store(false)
	if s.session == nil {
		return
	}
	s.session.Abort()
	s.cancelPending()
	s.session.ClearQueuedMessages()
}

func (s *sessionRuntime) close() {
	s.stop()
	if s.session != nil {
		s.session.Close()
	}
	if s.transport != nil {
		s.transport.Close()
	}
	if s.mcpLease != nil {
		s.mcpLease.Close()
	}
}

func (s *sessionRuntime) hasRunningTask() bool {
	if s.session == nil {
		return false
	}
	for _, task := range s.session.Tasks() {
		if task.Status == string(tools.TaskRunning) {
			return true
		}
	}
	return false
}

func (m *Manager) handleSessionEvent(sessionID string, runtime *sessionRuntime, ev engine.Event) {
	if ev.Type == engine.MessageCompleted || ev.Type == engine.CompactionCompleted {
		// Usage accounting must not interrupt a successful model run. The
		// transcript remains available for idempotent startup backfill if an
		// append fails transiently.
		_ = m.usage.RecordEvent(sessionID, ev)
	}
	if ev.Type == engine.UserMessageCompleted {
		if queued, found := runtime.consumePending(ev.QueueHandle); found {
			runtime.emit(MessageAccepted{
				ID:       queued.ID,
				Text:     ev.Text,
				Images:   ev.Images,
				Files:    ev.Files,
				SentAt:   ev.SentAt,
				Delivery: queued.Delivery,
			})
			// Message acceptance is observable before its background title state.
			m.maybeGenerateTitle(runtime, ev.Text)
			return
		}
		// Title generation is independent of the assistant run. Starting it
		// here means an interrupted first response can still receive an AI title.
		m.maybeGenerateTitle(runtime, ev.Text)
	}
	if ev.Type == engine.RunCompleted {
		runtime.live.Store(false)
	}
	runtime.forward(ev)
	if ev.Type == engine.TaskCompleted {
		// Task completion is delivered from the task manager's listener stack.
		// Release asynchronously so Session.Close never re-enters that manager.
		go m.ReleaseIfIdle(sessionID)
	}
}
