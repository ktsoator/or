package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/config"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

const (
	defaultTitle  = "New session"
	MaxTitleRunes = 120
	ScopeChat     = "chat"
	ScopeProject  = "project"
	KindScratch   = "scratch"
	KindFolder    = "folder"
)

// ErrSessionActive prevents deleting a conversation while its run or approval
// gate still owns live resources.
var ErrSessionActive = errors.New("session: session is running or waiting for approval")

// ErrImagesUnsupported rejects image attachments before a run is reserved
// when the session's selected model accepts text only.
var ErrImagesUnsupported = errors.New("session: selected model does not support images")

// ErrInvalidSessionScope reports a create request that is neither a standalone
// chat nor a project-backed conversation.
var ErrInvalidSessionScope = errors.New("session: invalid session scope")

// Summary is the browser-facing metadata for one independent coding
// conversation. Runtime-only state is sampled when the list is requested.
type Summary struct {
	ID            string                 `json:"id"`
	Title         string                 `json:"title"`
	AITitle       string                 `json:"aiTitle,omitempty"`
	CustomTitle   string                 `json:"customTitle,omitempty"`
	WorkspacePath string                 `json:"workspacePath"`
	WorkspaceName string                 `json:"workspaceName"`
	Scope         string                 `json:"scope"`
	WorkspaceKind string                 `json:"workspaceKind"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Running       bool                   `json:"running"`
	HasApproval   bool                   `json:"hasApproval"`
	ModelProvider string                 `json:"modelProvider"`
	ModelID       string                 `json:"modelId"`
	ModelName     string                 `json:"modelName"`
	ThinkingLevel llm.ModelThinkingLevel `json:"thinkingLevel"`
}

type record struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	AITitle       string    `json:"aiTitle,omitempty"`
	CustomTitle   string    `json:"customTitle,omitempty"`
	WorkspacePath string    `json:"workspacePath,omitempty"`
	Scope         string    `json:"scope,omitempty"`
	WorkspaceKind string    `json:"workspaceKind,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Transcript    string    `json:"transcript"`
	AutoTitle     bool      `json:"autoTitle,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	Thinking      string    `json:"thinkingLevel,omitempty"`
}

type Runtime struct {
	record    record
	session   *coding.Session
	transport Transport
	running   atomic.Bool

	pendingMu sync.Mutex
	pending   []QueuedMessage

	// titleGenerating is held only while an attempt is in flight, so a failed
	// attempt is retried on the next completed response.
	titleGenerating atomic.Bool
}

type Delivery string

const (
	DeliverySteer    Delivery = "steer"
	DeliveryFollowUp Delivery = "followup"
)

// Manager owns every conversation across the registered workspaces. Metadata
// is kept in indexes while each transcript and details sidecar stays separate.
// Lock ordering: mu is always taken before the workspace registry's own lock.
// The registry never calls back into this package, so that ordering holds
// simply by never taking mu inside a registry call.
type Manager struct {
	ctx        context.Context
	cfg        config.Config
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

// NewManager restores the session index. The ledger and registry are
// passed in rather than built here because the API layer serves them directly
// too; routing those reads through the manager only made it a facade for
// stores it does not own.
func NewManager(
	ctx context.Context,
	cfg config.Config,
	ledger *usage.Store,
	workspaces *workspace.Registry,
	newTransport NewTransport,
) (*Manager, error) {
	dir := filepath.Join(cfg.DataDir, "sessions")
	m := &Manager{
		ctx:          ctx,
		cfg:          cfg,
		indexPath:    filepath.Join(dir, "index.json"),
		scratch:      workspace.NewScratch(cfg.DataDir),
		workspaces:   workspaces,
		newTransport: newTransport,
		sessions:     make(map[string]*Runtime),
		usage:        ledger,
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

func (m *Manager) loadRecords() ([]record, error) {
	data, err := os.ReadFile(m.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: read session index: %w", err)
	}
	var records []record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("session: decode session index: %w", err)
	}
	return records, nil
}

func (m *Manager) build(record record) (*Runtime, error) {
	transport := m.newTransport(record.ID)
	cfg := m.cfg
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
	cfg.Cwd = workspacePath
	cfg.SessionFile = record.Transcript
	if record.Provider != "" {
		cfg.Provider = record.Provider
	}
	if record.Model != "" {
		cfg.Model = record.Model
	}
	if record.Thinking != "" {
		cfg.ThinkingLevel = record.Thinking
	}
	model, err := cfg.ResolveModel()
	if err != nil {
		return nil, err
	}
	thinking := llm.ClampThinkingLevel(model, cfg.Thinking())
	cfg.ThinkingLevel = string(thinking)
	record.Provider = model.Provider
	record.Model = model.ID
	record.Thinking = string(thinking)
	session, err := newCodingSession(m.ctx, cfg, transport.Confirm)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{record: record, session: session, transport: transport}
	session.Subscribe(func(ev coding.Event) {
		if ev.Type == coding.MessageCompleted || ev.Type == coding.CompactionCompleted {
			// Usage accounting must not interrupt a successful model run. The
			// transcript remains available for idempotent startup backfill if an
			// append fails transiently.
			_ = m.usage.RecordEvent(record.ID, ev)
		}
		if ev.Type == coding.MessageCompleted {
			// After the first final response, generate an AI title in the background.
			if ev.FinalResponse {
				m.maybeGenerateTitle(runtime)
			}
		}
		if ev.Type == coding.UserMessageCompleted {
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
		runtime.forward(ev)
	})
	if record.AutoTitle {
		for _, item := range session.History() {
			if item.Type == coding.HistoryUser && strings.TrimSpace(item.Text) != "" {
				runtime.record.Title = titleFromPrompt(item.Text)
				runtime.record.AutoTitle = false
				break
			}
		}
	}
	return runtime, nil
}

// Create adds an empty, independently persisted conversation. Chat sessions
// receive an isolated, manager-owned workspace; project sessions use the
// caller-selected folder and never fall back to the process working directory.
func (m *Manager) Create(
	title, workspacePath, scope string,
	model llm.Model,
	thinking llm.ModelThinkingLevel,
) (Summary, error) {
	startedAt := time.Now()
	now := startedAt.UTC()
	title = strings.TrimSpace(title)
	autoTitle := title == ""
	if autoTitle {
		title = defaultTitle
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := NewID()
	for m.sessions[id] != nil {
		id = NewID()
	}

	workspaceAdded := false
	workspaceCreated := false
	workspaceKind := ""
	switch scope {
	case ScopeChat:
		if strings.TrimSpace(workspacePath) != "" {
			return Summary{}, fmt.Errorf("%w: chat workspace is managed by the server", workspace.ErrInvalid)
		}
		var err error
		workspacePath, err = m.scratch.Create(id, startedAt)
		if err != nil {
			return Summary{}, err
		}
		workspaceCreated = true
		workspaceKind = KindScratch
	case ScopeProject:
		var err error
		workspacePath, err = workspace.Validate(workspacePath)
		if err != nil {
			return Summary{}, err
		}
		_, workspaceAdded = m.workspaces.Ensure(workspacePath, now)
		workspaceKind = KindFolder
	default:
		return Summary{}, fmt.Errorf("%w: %q", ErrInvalidSessionScope, scope)
	}

	record := record{
		ID:            id,
		Title:         title,
		WorkspacePath: workspacePath,
		Scope:         scope,
		WorkspaceKind: workspaceKind,
		CreatedAt:     now,
		UpdatedAt:     now,
		Transcript:    filepath.Join(filepath.Dir(m.indexPath), id+".jsonl"),
		AutoTitle:     autoTitle,
		Provider:      model.Provider,
		Model:         model.ID,
		Thinking:      string(llm.ClampThinkingLevel(model, thinking)),
	}
	runtime, err := m.build(record)
	if err != nil {
		if workspaceCreated {
			_ = m.scratch.Remove(workspacePath)
		}
		return Summary{}, err
	}
	m.sessions[id] = runtime
	if err := m.saveLocked(); err != nil {
		delete(m.sessions, id)
		if workspaceAdded {
			m.workspaces.Discard(workspacePath)
		}
		if workspaceCreated {
			_ = m.scratch.Remove(workspacePath)
		}
		return Summary{}, err
	}
	return runtime.summary(), nil
}

// Delete permanently removes one idle conversation and its persisted files.
// Files are staged under temporary names before the index is changed, so an
// index write failure can restore the conversation without data loss.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	runtime, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return os.ErrNotExist
	}
	if runtime.running.Load() || runtime.transport.HasPendingApproval() {
		m.mu.Unlock()
		return ErrSessionActive
	}

	paths, err := m.sessionFiles(runtime.record)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	staged, err := stageFiles(paths)
	if err != nil {
		m.mu.Unlock()
		return err
	}

	delete(m.sessions, id)
	if err := m.saveLocked(); err != nil {
		m.sessions[id] = runtime
		restoreFiles(staged)
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	// Stop any background shells the session started before its files go away.
	runtime.session.Close()

	for _, path := range staged {
		_ = removeStagedPath(path.staged)
	}
	// Reclaim the generated workspace only once the index no longer references
	// the session, so a failed index write above leaves the directory intact.
	// Folder workspaces belong to the user and are never touched; Scratch.Remove
	// re-proves the path is managed storage before it recurses.
	if runtime.record.WorkspaceKind == KindScratch {
		_ = m.scratch.Remove(runtime.record.WorkspacePath)
	}
	return nil
}

func (m *Manager) Get(id string) (*Runtime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runtime, ok := m.sessions[id]
	return runtime, ok
}

// UsesProvider reports whether any restored session currently references the
// provider. Keeping the active provider visible lets an installation manage
// its existing sessions even when credentials are supplied outside the
// process environment (for example by an upstream proxy).
func (m *Manager) UsesProvider(provider string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, runtime := range m.sessions {
		if runtime.record.Provider == provider {
			return true
		}
	}
	return false
}

// List returns newest-active first and samples each session's live state.
func (m *Manager) List() []Summary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Summary, 0, len(m.sessions))
	for _, runtime := range m.sessions {
		out = append(out, runtime.summary())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// UpdateSettings changes the model and reasoning effort used by the session's
// next prompt and persists the choice with that conversation.
func (m *Manager) UpdateSettings(
	id string,
	model llm.Model,
	thinking llm.ModelThinkingLevel,
) (Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return Summary{}, os.ErrNotExist
	}
	if runtime.running.Load() || runtime.transport.HasPendingApproval() {
		return Summary{}, ErrSessionActive
	}

	previousRecord := runtime.record
	previousModel, _ := llm.LookupModel(previousRecord.Provider, previousRecord.Model)
	previousThinking := llm.ModelThinkingLevel(previousRecord.Thinking)

	runtime.session.SetModel(model)
	runtime.session.SetThinkingLevel(thinking)
	runtime.record.Provider = model.Provider
	runtime.record.Model = model.ID
	runtime.record.Thinking = string(thinking)
	runtime.record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		runtime.record = previousRecord
		runtime.session.SetModel(previousModel)
		runtime.session.SetThinkingLevel(previousThinking)
		return Summary{}, err
	}
	return runtime.summary(), nil
}

// Rename sets a user-defined custom title on the session. An empty title clears
// the custom title so the display falls back to the AI or prompt-derived title.
func (m *Manager) Rename(id, customTitle string) (Summary, error) {
	customTitle = clampTitle(customTitle)
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return Summary{}, os.ErrNotExist
	}

	previousCustomTitle := runtime.record.CustomTitle
	runtime.record.CustomTitle = customTitle
	runtime.record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		runtime.record.CustomTitle = previousCustomTitle
		return Summary{}, err
	}
	runtime.broadcastTitle()
	return runtime.summary(), nil
}

// BeginPrompt reserves a session run and updates its durable title/activity.
func (m *Manager) BeginPrompt(id, prompt string, hasImages bool) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	if hasImages {
		model, found := llm.LookupModel(runtime.record.Provider, runtime.record.Model)
		if !found || !slices.Contains(model.Input, llm.Image) {
			return nil, ErrImagesUnsupported
		}
	}
	if !runtime.running.CompareAndSwap(false, true) {
		return nil, coding.ErrBusy
	}
	previous := runtime.record
	runtime.record.UpdatedAt = time.Now().UTC()
	if runtime.record.AutoTitle {
		title := prompt
		if strings.TrimSpace(title) == "" && hasImages {
			title = "Image"
		}
		runtime.record.Title = titleFromPrompt(title)
		runtime.record.AutoTitle = false
	}
	if err := m.saveLocked(); err != nil {
		runtime.record = previous
		runtime.running.Store(false)
		return nil, err
	}
	// Broadcast title change so the frontend updates immediately.
	runtime.broadcastTitle()
	return runtime, nil
}

// BeginCompact reserves an idle session for a manual context compaction. It
// does not alter the visible title or transcript; the coding Session commits
// the compaction boundary only after summary generation succeeds.
func (m *Manager) BeginCompact(id string) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	if runtime.transport.HasPendingApproval() || !runtime.running.CompareAndSwap(false, true) {
		return nil, ErrSessionActive
	}
	previous := runtime.record.UpdatedAt
	runtime.record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		runtime.record.UpdatedAt = previous
		runtime.running.Store(false)
		return nil, err
	}
	return runtime, nil
}

// EndRun clears live activity and records when the session last finished. The
// timestamp lets clients reject an older in-flight list response after an
// optimistic prompt update.
func (m *Manager) EndRun(id string) {
	m.mu.Lock()
	runtime, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	runtime.running.Store(false)
	runtime.record.UpdatedAt = time.Now().UTC()
	_ = m.saveLocked()
	m.mu.Unlock()

	cancelled := runtime.cancelPending()
	runtime.session.ClearQueuedMessages()
	for _, message := range cancelled {
		runtime.emit(MessageCancelled{ID: message.ID})
	}
}

func (m *Manager) saveLocked() error {
	// Both indexes move together: a session that registered a new workspace must
	// not be persisted while that workspace is missing from the sidebar.
	if err := m.workspaces.Save(); err != nil {
		return err
	}
	records := make([]record, 0, len(m.sessions))
	for _, runtime := range m.sessions {
		records = append(records, runtime.record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode session index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.indexPath), 0o755); err != nil {
		return fmt.Errorf("session: create session directory: %w", err)
	}
	tmp := m.indexPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("session: write session index: %w", err)
	}
	if err := os.Rename(tmp, m.indexPath); err != nil {
		return fmt.Errorf("session: replace session index: %w", err)
	}
	return nil
}

func (s *Runtime) summary() Summary {
	modelName := s.record.Model
	if model, ok := llm.LookupModel(s.record.Provider, s.record.Model); ok && model.Name != "" {
		modelName = model.Name
	}
	return Summary{
		ID:            s.record.ID,
		Title:         s.displayTitle(),
		AITitle:       s.record.AITitle,
		CustomTitle:   s.record.CustomTitle,
		WorkspacePath: s.record.WorkspacePath,
		WorkspaceName: filepath.Base(s.record.WorkspacePath),
		Scope:         s.record.Scope,
		WorkspaceKind: s.record.WorkspaceKind,
		CreatedAt:     s.record.CreatedAt,
		UpdatedAt:     s.record.UpdatedAt,
		Running:       s.running.Load(),
		HasApproval:   s.transport.HasPendingApproval(),
		ModelProvider: s.record.Provider,
		ModelID:       s.record.Model,
		ModelName:     modelName,
		ThinkingLevel: llm.ModelThinkingLevel(s.record.Thinking),
	}
}

// NewID returns an identifier for a session or a queued message.
func NewID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// Session exposes the underlying conversation for callers that drive a turn or
// read its transcript. The Runtime owns its lifecycle; callers only use it.
func (s *Runtime) Session() *coding.Session { return s.session }

// Running reports whether a turn is in flight.
func (s *Runtime) Running() bool { return s.running.Load() }

// HasPendingApproval reports a permission gate still waiting on an answer.
func (s *Runtime) HasPendingApproval() bool { return s.transport.HasPendingApproval() }
