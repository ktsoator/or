package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Create adds an empty, independently persisted conversation. Chat sessions
// receive an isolated, manager-owned workspace; project sessions use the
// caller-selected folder and never fall back to the process working directory.
func (m *Manager) Create(
	title, workspacePath, scope string,
	model llm.Model,
	thinking llm.ModelThinkingLevel,
	permissionMode permission.Mode,
) (Summary, error) {
	if !permissionMode.Valid() {
		return Summary{}, fmt.Errorf("%w: %q", ErrInvalidPermissionMode, permissionMode)
	}
	startedAt := time.Now()
	now := startedAt.UTC()
	title = strings.TrimSpace(title)
	autoTitle := title == ""
	if autoTitle {
		title = defaultTitle
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Summary{}, ErrManagerClosed
	}
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
		ID:             id,
		Title:          title,
		WorkspacePath:  workspacePath,
		Scope:          scope,
		WorkspaceKind:  workspaceKind,
		CreatedAt:      now,
		UpdatedAt:      now,
		Transcript:     filepath.Join(filepath.Dir(m.indexPath), id+".jsonl"),
		AutoTitle:      autoTitle,
		Provider:       model.Provider,
		Model:          model.ID,
		Thinking:       string(llm.ClampThinkingLevel(model, thinking)),
		PermissionMode: string(permissionMode),
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
		runtime.close()
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
	runtime.close()

	for _, path := range staged {
		_ = removeStagedPath(path.staged)
	}
	// Folder workspaces belong to the user and are never touched.
	if runtime.record.WorkspaceKind == KindScratch {
		_ = m.scratch.Remove(runtime.record.WorkspacePath)
	}
	return nil
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

// UpdatePermissionMode changes the baseline tool policy used by subsequent
// calls and persists it with the conversation.
func (m *Manager) UpdatePermissionMode(id string, mode permission.Mode) (Summary, error) {
	if !mode.Valid() {
		return Summary{}, fmt.Errorf("%w: %q", ErrInvalidPermissionMode, mode)
	}
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
	previousMode := permission.NormalizeMode(permission.Mode(previousRecord.PermissionMode))
	runtime.session.SetPermissionPolicy(permission.PolicyForMode(mode))
	runtime.record.PermissionMode = string(mode)
	runtime.record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		runtime.record = previousRecord
		runtime.session.SetPermissionPolicy(permission.PolicyForMode(previousMode))
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
