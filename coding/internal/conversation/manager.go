package conversation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Manager owns every conversation across the registered workspaces. Metadata
// is kept in indexes while each transcript and details sidecar stays separate.
// Lock ordering: mu is always taken before the workspace registry's own lock.
// The registry never calls back into this package, so that ordering holds
// simply by never taking mu inside a registry call.
type Manager struct {
	ctx        context.Context
	indexPath  string
	scratch    *workspace.Scratch
	workspaces *workspace.Registry
	// newTransport builds each session's link to its viewers. The delivery
	// layer supplies it, so this package never names a transport type.
	newTransport NewTransport

	mu       sync.RWMutex
	sessions map[string]*Runtime
	usage    *usage.Store
}

// Options supplies the product services and storage root owned by a Manager.
type Options struct {
	DataDir      string
	Usage        *usage.Store
	Workspaces   *workspace.Registry
	NewTransport NewTransport
}

// NewManager restores the session index. The ledger and registry are passed in
// because the HTTP layer also serves them directly.
func NewManager(ctx context.Context, opts Options) (*Manager, error) {
	dir := filepath.Join(opts.DataDir, "sessions")
	m := &Manager{
		ctx:          ctx,
		indexPath:    filepath.Join(dir, "index.json"),
		scratch:      workspace.NewScratch(opts.DataDir),
		workspaces:   opts.Workspaces,
		newTransport: opts.NewTransport,
		sessions:     make(map[string]*Runtime),
		usage:        opts.Usage,
	}

	records, err := m.loadRecords()
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		runtime, err := m.build(record)
		if err != nil {
			return nil, fmt.Errorf("session: restore session %s: %w", record.ID, err)
		}
		m.sessions[record.ID] = runtime
		if err := m.usage.BackfillEntries(record.ID, runtime.session.Entries()); err != nil {
			return nil, fmt.Errorf("session: backfill usage for session %s: %w", record.ID, err)
		}
	}
	if err := m.saveLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) build(record record) (*Runtime, error) {
	transport := m.newTransport(record.ID)
	if record.Scope != ScopeChat && record.Scope != ScopeProject {
		return nil, fmt.Errorf("session: invalid session scope %q", record.Scope)
	}
	if record.WorkspaceKind != KindScratch && record.WorkspaceKind != KindFolder {
		return nil, fmt.Errorf("session: invalid workspace kind %q", record.WorkspaceKind)
	}
	if record.Scope == ScopeChat && record.WorkspaceKind != KindScratch {
		return nil, fmt.Errorf("session: chat session requires a scratch workspace")
	}
	if record.Scope == ScopeProject && record.WorkspaceKind != KindFolder {
		return nil, fmt.Errorf("session: project session requires a folder workspace")
	}
	workspacePath, err := workspace.Clean(record.WorkspacePath)
	if err != nil {
		return nil, err
	}
	if record.WorkspaceKind == KindScratch {
		workspacePath, err = m.scratch.Validate(workspacePath)
		if err != nil {
			return nil, err
		}
		if err := workspace.EnsureDirectories(workspacePath); err != nil {
			return nil, err
		}
	}
	record.WorkspacePath = workspacePath
	model, ok := llm.LookupModel(record.Provider, record.Model)
	if !ok {
		return nil, fmt.Errorf("unknown model %q for provider %q", record.Model, record.Provider)
	}
	thinking := llm.ClampThinkingLevel(model, llm.ModelThinkingLevel(record.Thinking))
	record.Provider = model.Provider
	record.Model = model.ID
	record.Thinking = string(thinking)
	session, err := newEngineSession(m.ctx, engineSessionConfig{
		WorkspacePath:  workspacePath,
		TranscriptPath: record.Transcript,
		Model:          model,
		ThinkingLevel:  thinking,
	}, transport.Confirm)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{record: record, session: session, transport: transport}
	session.Subscribe(func(ev engine.Event) {
		if ev.Type == engine.MessageCompleted || ev.Type == engine.CompactionCompleted {
			// Usage accounting must not interrupt a successful model run. The
			// transcript remains available for idempotent startup backfill if an
			// append fails transiently.
			_ = m.usage.RecordEvent(record.ID, ev)
		}
		if ev.Type == engine.MessageCompleted && ev.FinalResponse {
			m.maybeGenerateTitle(runtime)
		}
		if ev.Type == engine.UserMessageCompleted {
			if queued, found := runtime.consumePending(ev.Text, ev.Images); found {
				runtime.emit(MessageAccepted{
					ID:       queued.ID,
					Text:     ev.Text,
					Images:   ev.Images,
					Delivery: queued.Delivery,
				})
				return
			}
		}
		if ev.Type == engine.RunCompleted {
			runtime.live.Store(false)
		}
		runtime.forward(ev)
	})
	if record.AutoTitle {
		for _, item := range session.History() {
			if item.Type == engine.HistoryUser && strings.TrimSpace(item.Text) != "" {
				runtime.record.Title = titleFromPrompt(item.Text)
				runtime.record.AutoTitle = false
				break
			}
		}
	}
	return runtime, nil
}
