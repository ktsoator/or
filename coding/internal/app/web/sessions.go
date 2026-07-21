package web

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
	"unicode/utf8"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/bootstrap"
	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/llm"
)

const (
	defaultSessionTitle  = "New session"
	maxTitleRunes        = 120
	sessionScopeChat     = "chat"
	sessionScopeProject  = "project"
	workspaceKindScratch = "scratch"
	workspaceKindFolder  = "folder"
)

// ErrSessionActive prevents deleting a conversation while its run or approval
// gate still owns live resources.
var ErrSessionActive = errors.New("web: session is running or waiting for approval")

// ErrImagesUnsupported rejects image attachments before a run is reserved
// when the session's selected model accepts text only.
var ErrImagesUnsupported = errors.New("web: selected model does not support images")

// ErrInvalidWorkspace reports a path that cannot be used as a workspace root.
var ErrInvalidWorkspace = errors.New("web: invalid workspace")

// ErrInvalidSessionScope reports a create request that is neither a standalone
// chat nor a project-backed conversation.
var ErrInvalidSessionScope = errors.New("web: invalid session scope")

// SessionSummary is the browser-facing metadata for one independent coding
// conversation. Runtime-only state is sampled when the list is requested.
type SessionSummary struct {
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

// WorkspaceSummary is a registered project root. Workspaces are persisted
// independently from sessions so an empty project can remain in the sidebar.
type WorkspaceSummary struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"addedAt"`
}

type workspaceRecord struct {
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
}

type sessionRecord struct {
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

type sessionRuntime struct {
	record  sessionRecord
	session *coding.Session
	hub     *Hub
	broker  *ConfirmBroker
	running atomic.Bool

	pendingMu sync.Mutex
	pending   []queuedMessage

	// titleGenerating is held only while an attempt is in flight, so a failed
	// attempt is retried on the next completed response.
	titleGenerating atomic.Bool
}

type queuedDelivery string

const (
	deliverySteer    queuedDelivery = "steer"
	deliveryFollowUp queuedDelivery = "followup"
)

type queuedMessage struct {
	ID       string
	Delivery queuedDelivery
	Text     string
	Images   []llm.ImageContent
	Handle   coding.QueueHandle
}

// SessionManager owns web sessions across registered workspaces. Metadata is
// kept in indexes while every transcript and details sidecar remains separate.
type SessionManager struct {
	ctx                context.Context
	cfg                config.Config
	indexPath          string
	workspaceIndexPath string

	mu         sync.RWMutex
	sessions   map[string]*sessionRuntime
	workspaces map[string]workspaceRecord
	usage      *UsageStore
}

// NewSessionManager restores the global Web session and workspace indexes.
func NewSessionManager(ctx context.Context, cfg config.Config) (*SessionManager, error) {
	dir := filepath.Join(cfg.DataDir, "sessions")
	usage, err := NewUsageStore(filepath.Join(cfg.DataDir, "usage", "events.jsonl"))
	if err != nil {
		return nil, err
	}
	m := &SessionManager{
		ctx:                ctx,
		cfg:                cfg,
		indexPath:          filepath.Join(dir, "index.json"),
		workspaceIndexPath: filepath.Join(dir, "workspaces.json"),
		sessions:           make(map[string]*sessionRuntime),
		workspaces:         make(map[string]workspaceRecord),
		usage:              usage,
	}

	workspaceRecords, err := m.loadWorkspaceRecords()
	if err != nil {
		return nil, err
	}
	for _, record := range workspaceRecords {
		path, cleanErr := cleanWorkspacePath(record.Path)
		if cleanErr != nil {
			continue
		}
		record.Path = path
		m.workspaces[path] = record
	}

	records, err := m.loadRecords()
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		runtime, err := m.build(record)
		if err != nil {
			return nil, fmt.Errorf("web: restore session %s: %w", record.ID, err)
		}
		m.sessions[record.ID] = runtime
		if err := m.usage.Backfill(record.ID, runtime.session.Messages()); err != nil {
			return nil, fmt.Errorf("web: backfill usage for session %s: %w", record.ID, err)
		}
	}
	if err := m.saveLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *SessionManager) loadWorkspaceRecords() ([]workspaceRecord, error) {
	data, err := os.ReadFile(m.workspaceIndexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("web: read workspace index: %w", err)
	}
	var records []workspaceRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("web: decode workspace index: %w", err)
	}
	return records, nil
}

func (m *SessionManager) loadRecords() ([]sessionRecord, error) {
	data, err := os.ReadFile(m.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("web: read session index: %w", err)
	}
	var records []sessionRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("web: decode session index: %w", err)
	}
	return records, nil
}

func (m *SessionManager) build(record sessionRecord) (*sessionRuntime, error) {
	hub := NewHub()
	broker := NewConfirmBroker(hub)
	cfg := m.cfg
	if record.Scope != sessionScopeChat && record.Scope != sessionScopeProject {
		return nil, fmt.Errorf("web: invalid session scope %q", record.Scope)
	}
	if record.WorkspaceKind != workspaceKindScratch && record.WorkspaceKind != workspaceKindFolder {
		return nil, fmt.Errorf("web: invalid workspace kind %q", record.WorkspaceKind)
	}
	if record.Scope == sessionScopeChat && record.WorkspaceKind != workspaceKindScratch {
		return nil, fmt.Errorf("web: chat session requires a scratch workspace")
	}
	if record.Scope == sessionScopeProject && record.WorkspaceKind != workspaceKindFolder {
		return nil, fmt.Errorf("web: project session requires a folder workspace")
	}
	workspacePath, err := cleanWorkspacePath(record.WorkspacePath)
	if err != nil {
		return nil, err
	}
	if record.WorkspaceKind == workspaceKindScratch {
		workspacePath, err = m.validateScratchWorkspacePath(workspacePath)
		if err != nil {
			return nil, err
		}
		if err := ensureScratchWorkspaceDirectories(workspacePath); err != nil {
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
	session, err := bootstrap.NewSession(m.ctx, cfg, bootstrap.Dependencies{Confirm: broker.Confirm})
	if err != nil {
		return nil, err
	}
	runtime := &sessionRuntime{record: record, session: session, hub: hub, broker: broker}
	session.Subscribe(func(ev coding.Event) {
		if ev.Type == coding.MessageCompleted {
			// Usage accounting must not interrupt a successful model run. The
			// transcript remains available for idempotent startup backfill if an
			// append fails transiently.
			_ = m.usage.RecordEvent(record.ID, ev)
			// After the first final response, generate an AI title in the background.
			if ev.FinalResponse {
				m.maybeGenerateTitle(runtime)
			}
		}
		if ev.Type == coding.UserMessageCompleted {
			if queued, found := runtime.consumePending(ev.Text, ev.Images); found {
				data, _ := json.Marshal(wireEvent{
					Type:     "user_message",
					ID:       queued.ID,
					Text:     ev.Text,
					Images:   projectImages(ev.Images),
					Delivery: string(queued.Delivery),
				})
				hub.Broadcast(data)
				return
			}
		}
		if data, ok := ProjectEvent(ev); ok {
			hub.Broadcast(data)
		}
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
func (m *SessionManager) Create(
	title, workspacePath, scope string,
	model llm.Model,
	thinking llm.ModelThinkingLevel,
) (SessionSummary, error) {
	startedAt := time.Now()
	now := startedAt.UTC()
	title = strings.TrimSpace(title)
	autoTitle := title == ""
	if autoTitle {
		title = defaultSessionTitle
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := newSessionID()
	for m.sessions[id] != nil {
		id = newSessionID()
	}

	workspaceAdded := false
	workspaceCreated := false
	workspaceKind := ""
	switch scope {
	case sessionScopeChat:
		if strings.TrimSpace(workspacePath) != "" {
			return SessionSummary{}, fmt.Errorf("%w: chat workspace is managed by the server", ErrInvalidWorkspace)
		}
		var err error
		workspacePath, err = m.createScratchWorkspace(id, startedAt)
		if err != nil {
			return SessionSummary{}, err
		}
		workspaceCreated = true
		workspaceKind = workspaceKindScratch
	case sessionScopeProject:
		var err error
		workspacePath, err = validateWorkspacePath(workspacePath)
		if err != nil {
			return SessionSummary{}, err
		}
		_, workspaceAdded = m.ensureWorkspaceLocked(workspacePath, now)
		workspaceKind = workspaceKindFolder
	default:
		return SessionSummary{}, fmt.Errorf("%w: %q", ErrInvalidSessionScope, scope)
	}

	record := sessionRecord{
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
			_ = m.removeScratchWorkspace(workspacePath)
		}
		return SessionSummary{}, err
	}
	m.sessions[id] = runtime
	if err := m.saveLocked(); err != nil {
		delete(m.sessions, id)
		if workspaceAdded {
			delete(m.workspaces, workspacePath)
		}
		if workspaceCreated {
			_ = m.removeScratchWorkspace(workspacePath)
		}
		return SessionSummary{}, err
	}
	return runtime.summary(), nil
}

// RegisterWorkspace persists a project root without creating a conversation.
func (m *SessionManager) RegisterWorkspace(path string) (WorkspaceSummary, error) {
	cleaned, err := validateWorkspacePath(path)
	if err != nil {
		return WorkspaceSummary{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.workspaces[cleaned]; ok {
		return existing.summary(), nil
	}
	record := workspaceRecord{Path: cleaned, AddedAt: time.Now().UTC()}
	m.workspaces[cleaned] = record
	if err := m.saveWorkspacesLocked(); err != nil {
		delete(m.workspaces, cleaned)
		return WorkspaceSummary{}, err
	}
	return record.summary(), nil
}

// RemoveWorkspace removes a project from the registered sidebar list. Session
// transcripts and workspace files are intentionally retained; registering the
// same directory again makes its existing sessions visible again.
func (m *SessionManager) RemoveWorkspace(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: path is required", ErrInvalidWorkspace)
	}
	cleaned, err := cleanWorkspacePath(path)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.workspaces[cleaned]
	if !ok {
		return nil
	}
	delete(m.workspaces, cleaned)
	if err := m.saveWorkspacesLocked(); err != nil {
		m.workspaces[cleaned] = record
		return err
	}
	return nil
}

// ListWorkspaces returns registered projects newest-added first, including
// projects that currently have no conversations.
func (m *SessionManager) ListWorkspaces() []WorkspaceSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]WorkspaceSummary, 0, len(m.workspaces))
	for _, record := range m.workspaces {
		out = append(out, record.summary())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].AddedAt.After(out[j].AddedAt)
	})
	return out
}

// UsageReport returns the durable aggregate across conversations since the cutoff.
func (m *SessionManager) UsageReport(since time.Time) UsageReport {
	return m.usage.Report(since)
}

// UsageEvents returns paginated per-request usage details.
func (m *SessionManager) UsageEvents(provider, model string, since time.Time, offset, limit int) UsageEventPage {
	return m.usage.Events(provider, model, since, offset, limit)
}

func (m *SessionManager) ensureWorkspaceLocked(path string, addedAt time.Time) (workspaceRecord, bool) {
	if existing, ok := m.workspaces[path]; ok {
		return existing, false
	}
	if addedAt.IsZero() {
		addedAt = time.Now().UTC()
	}
	record := workspaceRecord{Path: path, AddedAt: addedAt}
	m.workspaces[path] = record
	return record, true
}

// Delete permanently removes one idle conversation and its persisted files.
// Files are staged under temporary names before the index is changed, so an
// index write failure can restore the conversation without data loss.
func (m *SessionManager) Delete(id string) error {
	m.mu.Lock()
	runtime, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return os.ErrNotExist
	}
	if runtime.running.Load() || runtime.broker.HasPending() {
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
	// Folder workspaces belong to the user and are never touched;
	// removeScratchWorkspace re-proves the path is managed storage before it
	// recurses.
	if runtime.record.WorkspaceKind == workspaceKindScratch {
		_ = m.removeScratchWorkspace(runtime.record.WorkspacePath)
	}
	return nil
}

func (m *SessionManager) Get(id string) (*sessionRuntime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runtime, ok := m.sessions[id]
	return runtime, ok
}

// UsesProvider reports whether any restored session currently references the
// provider. Keeping the active provider visible lets an installation manage
// its existing sessions even when credentials are supplied outside the
// process environment (for example by an upstream proxy).
func (m *SessionManager) UsesProvider(provider string) bool {
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
func (m *SessionManager) List() []SessionSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SessionSummary, 0, len(m.sessions))
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
func (m *SessionManager) UpdateSettings(
	id string,
	model llm.Model,
	thinking llm.ModelThinkingLevel,
) (SessionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return SessionSummary{}, os.ErrNotExist
	}
	if runtime.running.Load() || runtime.broker.HasPending() {
		return SessionSummary{}, ErrSessionActive
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
		return SessionSummary{}, err
	}
	return runtime.summary(), nil
}

// Rename sets a user-defined custom title on the session. An empty title clears
// the custom title so the display falls back to the AI or prompt-derived title.
func (m *SessionManager) Rename(id, customTitle string) (SessionSummary, error) {
	customTitle = clampTitle(customTitle)
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return SessionSummary{}, os.ErrNotExist
	}

	previousCustomTitle := runtime.record.CustomTitle
	runtime.record.CustomTitle = customTitle
	runtime.record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		runtime.record.CustomTitle = previousCustomTitle
		return SessionSummary{}, err
	}
	runtime.broadcastTitle()
	return runtime.summary(), nil
}

// BeginPrompt reserves a session run and updates its durable title/activity.
func (m *SessionManager) BeginPrompt(id, prompt string, hasImages bool) (*sessionRuntime, error) {
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

// EndRun clears live activity and records when the session last finished. The
// timestamp lets clients reject an older in-flight list response after an
// optimistic prompt update.
func (m *SessionManager) EndRun(id string) {
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
		data, _ := json.Marshal(wireEvent{Type: "queue_cancelled", ID: message.ID})
		runtime.hub.Broadcast(data)
	}
}

func (m *SessionManager) saveLocked() error {
	if err := m.saveWorkspacesLocked(); err != nil {
		return err
	}
	records := make([]sessionRecord, 0, len(m.sessions))
	for _, runtime := range m.sessions {
		records = append(records, runtime.record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("web: encode session index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.indexPath), 0o755); err != nil {
		return fmt.Errorf("web: create session directory: %w", err)
	}
	tmp := m.indexPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("web: write session index: %w", err)
	}
	if err := os.Rename(tmp, m.indexPath); err != nil {
		return fmt.Errorf("web: replace session index: %w", err)
	}
	return nil
}

func (m *SessionManager) saveWorkspacesLocked() error {
	records := make([]workspaceRecord, 0, len(m.workspaces))
	for _, record := range m.workspaces {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].AddedAt.Before(records[j].AddedAt) })
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("web: encode workspace index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.workspaceIndexPath), 0o755); err != nil {
		return fmt.Errorf("web: create workspace directory: %w", err)
	}
	tmp := m.workspaceIndexPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("web: write workspace index: %w", err)
	}
	if err := os.Rename(tmp, m.workspaceIndexPath); err != nil {
		return fmt.Errorf("web: replace workspace index: %w", err)
	}
	return nil
}

type stagedFile struct {
	original string
	staged   string
}

func (m *SessionManager) sessionFiles(record sessionRecord) ([]string, error) {
	transcript, err := filepath.Abs(record.Transcript)
	if err != nil {
		return nil, err
	}
	sessionDir, err := filepath.Abs(filepath.Dir(m.indexPath))
	if err != nil {
		return nil, err
	}
	if filepath.Dir(transcript) != sessionDir {
		return nil, fmt.Errorf("web: refusing to delete transcript outside session storage: %s", transcript)
	}
	details := strings.TrimSuffix(transcript, ".jsonl") + ".details.jsonl"
	return []string{transcript, details}, nil
}

func stageFiles(paths []string) ([]stagedFile, error) {
	var staged []stagedFile
	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			restoreFiles(staged)
			return nil, err
		}
		tombstone := path + ".deleted-" + newSessionID()
		if err := os.Rename(path, tombstone); err != nil {
			restoreFiles(staged)
			return nil, err
		}
		staged = append(staged, stagedFile{original: path, staged: tombstone})
	}
	return staged, nil
}

func restoreFiles(files []stagedFile) {
	for i := len(files) - 1; i >= 0; i-- {
		_ = os.Rename(files[i].staged, files[i].original)
	}
}

func removeStagedPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func (s *sessionRuntime) summary() SessionSummary {
	modelName := s.record.Model
	if model, ok := llm.LookupModel(s.record.Provider, s.record.Model); ok && model.Name != "" {
		modelName = model.Name
	}
	return SessionSummary{
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
		HasApproval:   s.broker.HasPending(),
		ModelProvider: s.record.Provider,
		ModelID:       s.record.Model,
		ModelName:     modelName,
		ThinkingLevel: llm.ModelThinkingLevel(s.record.Thinking),
	}
}

func (w workspaceRecord) summary() WorkspaceSummary {
	return WorkspaceSummary{
		Path:    w.Path,
		Name:    filepath.Base(w.Path),
		AddedAt: w.AddedAt,
	}
}

func cleanWorkspacePath(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidWorkspace, err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

func validateWorkspacePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidWorkspace)
	}
	cleaned, err := cleanWorkspacePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidWorkspace, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", ErrInvalidWorkspace, cleaned)
	}
	return cleaned, nil
}

func (m *SessionManager) scratchWorkspaceRoot() string {
	return filepath.Join(m.cfg.DataDir, "workspaces")
}

// validateScratchWorkspacePath proves that path is one generated session
// directory exactly two levels below the managed workspace root. This guard is
// required before any recursive cleanup, so an external project can never be
// removed through forged session metadata.
func (m *SessionManager) validateScratchWorkspacePath(path string) (string, error) {
	root, err := cleanWorkspacePath(m.scratchWorkspaceRoot())
	if err != nil {
		return "", err
	}
	cleaned, err := cleanWorkspacePath(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, cleaned)
	if err != nil || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: scratch workspace is outside managed storage", ErrInvalidWorkspace)
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == ".." {
		return "", fmt.Errorf("%w: invalid scratch workspace path", ErrInvalidWorkspace)
	}
	return cleaned, nil
}

func (m *SessionManager) createScratchWorkspace(id string, startedAt time.Time) (string, error) {
	path := filepath.Join(m.scratchWorkspaceRoot(), startedAt.Format("2006-01-02"), id)
	if err := ensureScratchWorkspaceDirectories(path); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	cleaned, err := m.validateScratchWorkspacePath(path)
	if err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	return cleaned, nil
}

func ensureScratchWorkspaceDirectories(path string) error {
	for _, directory := range []string{path, filepath.Join(path, "work"), filepath.Join(path, "outputs")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("web: create scratch workspace: %w", err)
		}
	}
	return nil
}

func (m *SessionManager) removeScratchWorkspace(path string) error {
	cleaned, err := m.validateScratchWorkspacePath(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(cleaned)
}

func (s *sessionRuntime) queuePending(message queuedMessage) bool {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if !s.running.Load() {
		return false
	}
	if message.Delivery == deliverySteer {
		message.Handle = s.session.Steer(message.Text, message.Images...)
	} else {
		message.Handle = s.session.FollowUp(message.Text, message.Images...)
	}
	s.pending = append(s.pending, message)
	data, _ := json.Marshal(wireEvent{
		Type:     "user_message",
		ID:       message.ID,
		Text:     message.Text,
		Images:   projectImages(message.Images),
		Delivery: string(message.Delivery),
		Queued:   true,
	})
	s.hub.Broadcast(data)
	return true
}

func (s *sessionRuntime) removePending(id string) (found, removed bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for index, message := range s.pending {
		if message.ID != id {
			continue
		}
		if !s.session.CancelQueuedMessage(message.Handle) {
			return true, false
		}
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		data, _ := json.Marshal(wireEvent{Type: "queue_removed", ID: id})
		s.hub.Broadcast(data)
		return true, true
	}
	return false, false
}

func (s *sessionRuntime) consumePending(text string, images []llm.ImageContent) (queuedMessage, bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for index, message := range s.pending {
		if message.Text != text || !sameImages(message.Images, images) {
			continue
		}
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		return message, true
	}
	return queuedMessage{}, false
}

func (s *sessionRuntime) pendingEvents() []wireEvent {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	events := make([]wireEvent, 0, len(s.pending))
	for _, message := range s.pending {
		events = append(events, wireEvent{
			Type:     "user_message",
			ID:       message.ID,
			Text:     message.Text,
			Images:   projectImages(message.Images),
			Delivery: string(message.Delivery),
			Queued:   true,
		})
	}
	return events
}

func (s *sessionRuntime) cancelPending() []queuedMessage {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	cancelled := append([]queuedMessage(nil), s.pending...)
	s.pending = nil
	return cancelled
}

func sameImages(left, right []llm.ImageContent) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].MIMEType != right[index].MIMEType || left[index].Data != right[index].Data {
			return false
		}
	}
	return true
}

func newSessionID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func titleFromPrompt(prompt string) string {
	title := strings.Join(strings.Fields(prompt), " ")
	if title == "" {
		return defaultSessionTitle
	}
	const maxRunes = 42
	if utf8.RuneCountInString(title) <= maxRunes {
		return title
	}
	runes := []rune(title)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

// clampTitle trims a title and caps it at maxTitleRunes so a long model or
// client-supplied string cannot bloat the session store.
func clampTitle(title string) string {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) <= maxTitleRunes {
		return title
	}
	runes := []rune(title)
	return strings.TrimSpace(string(runes[:maxTitleRunes]))
}

// displayTitle returns the best available title for this session. Callers must
// hold SessionManager.mu.
func (s *sessionRuntime) displayTitle() string {
	if s.record.CustomTitle != "" {
		return s.record.CustomTitle
	}
	if s.record.AITitle != "" {
		return s.record.AITitle
	}
	return s.record.Title
}

// broadcastTitle sends the current title to connected clients. Callers must hold
// SessionManager.mu; Hub.Broadcast never blocks, so holding it is cheap.
func (s *sessionRuntime) broadcastTitle() {
	data, _ := json.Marshal(wireEvent{
		Type:        "title_update",
		Title:       s.displayTitle(),
		AITitle:     s.record.AITitle,
		CustomTitle: s.record.CustomTitle,
	})
	s.hub.Broadcast(data)
}

// maybeGenerateTitle starts background AI title generation after a session
// finishes a response, unless the user has already named it or a title was
// generated earlier. The flag only guards against two generations running at
// once: a failed attempt clears it so the next completed response retries,
// because a model error or an unparseable reply should not cost the session its
// title for the lifetime of the process. Runs on the session's event goroutine,
// so it must not block on the model call.
func (m *SessionManager) maybeGenerateTitle(runtime *sessionRuntime) {
	m.mu.Lock()
	needsTitle := runtime.record.CustomTitle == "" && runtime.record.AITitle == ""
	provider, model := runtime.record.Provider, runtime.record.Model
	m.mu.Unlock()

	if !needsTitle || !runtime.titleGenerating.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer runtime.titleGenerating.Store(false)
		m.generateSessionTitle(m.ctx, runtime, provider, model)
	}()
}

// generateSessionTitle asks the model for a concise session title derived from
// the first user message and stores it as the session's AI title. Failures are
// silent: the session keeps its prompt-derived title.
func (m *SessionManager) generateSessionTitle(ctx context.Context, runtime *sessionRuntime, provider, modelID string) {
	// Find the first user message with text content.
	history := runtime.session.History()
	var firstPrompt string
	for _, item := range history {
		if item.Type == coding.HistoryUser && strings.TrimSpace(item.Text) != "" {
			firstPrompt = strings.TrimSpace(item.Text)
			break
		}
	}
	if firstPrompt == "" {
		return
	}

	model, ok := llm.LookupModel(provider, modelID)
	if !ok {
		return
	}

	systemPrompt := `Generate a concise, sentence-case title (3-7 words) that captures the main topic of the user's first message. Use sentence case: capitalize only the first word and proper nouns. Return JSON with a single "title" field.

Good examples:
{"title": "Fix login button on mobile"}
{"title": "Add OAuth authentication"}
{"title": "Debug failing CI tests"}

Bad (too vague): {"title": "Code changes"}
Bad (too long): {"title": "Investigate and fix the issue with the login flow"}`

	titleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Thinking is off: a title needs none, and reasoning tokens would consume
	// the output budget before any JSON is emitted.
	result, err := llm.Complete(titleCtx, model, llm.PromptWithSystem(systemPrompt, firstPrompt), llm.StreamOptions{
		MaxTokens: 128,
		Reasoning: llm.ModelThinkingOff,
	})
	if err != nil {
		return
	}

	title := clampTitle(parseTitleJSON(result.Text()))
	if title == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Re-check under the lock: the user may have renamed while we generated.
	if runtime.record.CustomTitle != "" {
		return
	}
	runtime.record.AITitle = title
	if err := m.saveLocked(); err != nil {
		runtime.record.AITitle = ""
		return
	}
	runtime.broadcastTitle()
}

// parseTitleJSON pulls the title out of a model response. It accepts the JSON
// object the prompt asks for, the same object wrapped in prose or a code fence,
// and — because smaller models often ignore the format — a bare one-line title.
func parseTitleJSON(text string) string {
	var parsed struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return parsed.Title
	}
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		if json.Unmarshal([]byte(text[start:end+1]), &parsed) == nil {
			return parsed.Title
		}
	}
	// Bare title: a single short line with no JSON in sight. Anything longer or
	// multi-line is prose, not a title, so it is discarded.
	line := strings.TrimSpace(text)
	if strings.ContainsAny(line, "{}\n\r") || utf8.RuneCountInString(line) > maxTitleRunes {
		return ""
	}
	return strings.Trim(line, `"'`)
}
