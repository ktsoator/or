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

const defaultSessionTitle = "New session"

// ErrSessionActive prevents deleting a conversation while its run or approval
// gate still owns live resources.
var ErrSessionActive = errors.New("web: session is running or waiting for approval")

// ErrImagesUnsupported rejects image attachments before a run is reserved
// when the session's selected model accepts text only.
var ErrImagesUnsupported = errors.New("web: selected model does not support images")

// SessionSummary is the browser-facing metadata for one independent coding
// conversation. Runtime-only state is sampled when the list is requested.
type SessionSummary struct {
	ID            string                 `json:"id"`
	Title         string                 `json:"title"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Running       bool                   `json:"running"`
	HasApproval   bool                   `json:"hasApproval"`
	ModelProvider string                 `json:"modelProvider"`
	ModelID       string                 `json:"modelId"`
	ModelName     string                 `json:"modelName"`
	ThinkingLevel llm.ModelThinkingLevel `json:"thinkingLevel"`
}

type sessionRecord struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Transcript string    `json:"transcript"`
	AutoTitle  bool      `json:"autoTitle,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Model      string    `json:"model,omitempty"`
	Thinking   string    `json:"thinkingLevel,omitempty"`
}

type sessionRuntime struct {
	record  sessionRecord
	session *coding.Session
	hub     *Hub
	broker  *ConfirmBroker
	running atomic.Bool
}

// SessionManager owns all web sessions for one workspace. Metadata is kept in
// an index while every transcript and details sidecar remains a separate file.
type SessionManager struct {
	ctx       context.Context
	cfg       config.Config
	indexPath string

	mu       sync.RWMutex
	sessions map[string]*sessionRuntime
}

// NewSessionManager restores the workspace's session index. An existing
// single-session transcript is adopted as the first conversation on upgrade.
func NewSessionManager(ctx context.Context, cfg config.Config) (*SessionManager, error) {
	dir := filepath.Join(filepath.Dir(cfg.SessionFile), "sessions")
	m := &SessionManager{
		ctx:       ctx,
		cfg:       cfg,
		indexPath: filepath.Join(dir, "index.json"),
		sessions:  make(map[string]*sessionRuntime),
	}

	records, err := m.loadRecords()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		records = []sessionRecord{m.initialRecord()}
	}
	for _, record := range records {
		runtime, err := m.build(record)
		if err != nil {
			return nil, fmt.Errorf("web: restore session %s: %w", record.ID, err)
		}
		m.sessions[record.ID] = runtime
	}
	if err := m.saveLocked(); err != nil {
		return nil, err
	}
	return m, nil
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

func (m *SessionManager) initialRecord() sessionRecord {
	now := time.Now().UTC()
	if info, err := os.Stat(m.cfg.SessionFile); err == nil {
		now = info.ModTime().UTC()
	}
	return sessionRecord{
		ID:         newSessionID(),
		Title:      defaultSessionTitle,
		CreatedAt:  now,
		UpdatedAt:  now,
		Transcript: m.cfg.SessionFile,
		AutoTitle:  true,
	}
}

func (m *SessionManager) build(record sessionRecord) (*sessionRuntime, error) {
	hub := NewHub()
	broker := NewConfirmBroker(hub)
	cfg := m.cfg
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

// Create adds an empty, independently persisted conversation.
func (m *SessionManager) Create(title string) (SessionSummary, error) {
	now := time.Now().UTC()
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
	record := sessionRecord{
		ID:         id,
		Title:      title,
		CreatedAt:  now,
		UpdatedAt:  now,
		Transcript: filepath.Join(filepath.Dir(m.indexPath), id+".jsonl"),
		AutoTitle:  autoTitle,
	}
	runtime, err := m.build(record)
	if err != nil {
		return SessionSummary{}, err
	}
	m.sessions[id] = runtime
	if err := m.saveLocked(); err != nil {
		delete(m.sessions, id)
		return SessionSummary{}, err
	}
	return runtime.summary(), nil
}

// Delete permanently removes one idle conversation and its persisted files.
// Files are staged under temporary names before the index is changed, so an
// index write failure can restore the conversation without data loss. Deleting
// the final conversation creates a fresh empty replacement.
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
	var replacementID string
	if len(m.sessions) == 0 {
		replacementID = newSessionID()
		now := time.Now().UTC()
		record := sessionRecord{
			ID:         replacementID,
			Title:      defaultSessionTitle,
			CreatedAt:  now,
			UpdatedAt:  now,
			Transcript: filepath.Join(filepath.Dir(m.indexPath), replacementID+".jsonl"),
			AutoTitle:  true,
		}
		replacement, buildErr := m.build(record)
		if buildErr != nil {
			m.sessions[id] = runtime
			restoreFiles(staged)
			m.mu.Unlock()
			return buildErr
		}
		m.sessions[replacementID] = replacement
	}
	if err := m.saveLocked(); err != nil {
		delete(m.sessions, replacementID)
		m.sessions[id] = runtime
		restoreFiles(staged)
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	for _, file := range staged {
		_ = os.Remove(file.staged)
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
	return runtime, nil
}

// EndRun clears live activity and records when the session last finished. The
// timestamp lets clients reject an older in-flight list response after an
// optimistic prompt update.
func (m *SessionManager) EndRun(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.sessions[id]
	if !ok {
		return
	}
	runtime.running.Store(false)
	runtime.record.UpdatedAt = time.Now().UTC()
	_ = m.saveLocked()
}

func (m *SessionManager) saveLocked() error {
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

type stagedFile struct {
	original string
	staged   string
}

func (m *SessionManager) sessionFiles(record sessionRecord) ([]string, error) {
	transcript, err := filepath.Abs(record.Transcript)
	if err != nil {
		return nil, err
	}
	legacy, err := filepath.Abs(m.cfg.SessionFile)
	if err != nil {
		return nil, err
	}
	sessionDir, err := filepath.Abs(filepath.Dir(m.indexPath))
	if err != nil {
		return nil, err
	}
	if transcript != legacy && filepath.Dir(transcript) != sessionDir {
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

func (s *sessionRuntime) summary() SessionSummary {
	modelName := s.record.Model
	if model, ok := llm.LookupModel(s.record.Provider, s.record.Model); ok && model.Name != "" {
		modelName = model.Name
	}
	return SessionSummary{
		ID:            s.record.ID,
		Title:         s.record.Title,
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
