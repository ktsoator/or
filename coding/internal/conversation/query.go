package conversation

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

// Snapshot is the complete client-readable state of one conversation.
type Snapshot struct {
	History      []engine.HistoryItem
	Queue        []Event
	ContextUsage engine.ContextUsage
	Tasks        []engine.BackgroundTask
	Running      bool
	Title        string
}

// Snapshot returns the current client-readable state without exposing the
// runtime that owns the engine session.
func (m *Manager) Snapshot(id string) (Snapshot, error) {
	m.mu.Lock()
	runtime, err := m.loadRuntimeLocked(id)
	if err != nil {
		m.mu.Unlock()
		return Snapshot{}, err
	}
	title := runtime.displayTitle()
	m.mu.Unlock()
	return Snapshot{
		History:      runtime.session.History(),
		Queue:        runtime.pendingEvents(),
		ContextUsage: runtime.session.ContextUsage(),
		Tasks:        runtime.session.Tasks(),
		Running:      runtime.live.Load(),
		Title:        title,
	}, nil
}

// StopTask terminates one background task owned by the conversation.
func (m *Manager) StopTask(sessionID, taskID string) error {
	m.mu.Lock()
	runtime, err := m.loadRuntimeLocked(sessionID)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	return runtime.session.StopTask(taskID)
}

// TaskOutput returns a bounded tail of one conversation task's logs.
func (m *Manager) TaskOutput(sessionID, taskID string) (engine.TaskOutput, error) {
	m.mu.Lock()
	runtime, err := m.loadRuntimeLocked(sessionID)
	m.mu.Unlock()
	if err != nil {
		return engine.TaskOutput{}, err
	}
	return runtime.session.TaskOutput(taskID)
}

// WorkspacePath returns the tool root owned by one conversation.
func (m *Manager) WorkspacePath(id string) (string, error) {
	m.mu.RLock()
	runtime, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return "", os.ErrNotExist
	}
	return runtime.record.WorkspacePath, nil
}

// Abort cancels the active run, if any.
func (m *Manager) Abort(id string) error {
	m.mu.RLock()
	runtime, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return os.ErrNotExist
	}
	if runtime.session != nil {
		runtime.session.Abort()
	}
	return nil
}

// runtime returns the package-owned runtime for internal coordination and
// white-box tests. It is intentionally not exposed to product adapters.
func (m *Manager) runtime(id string) (*sessionRuntime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runtime, ok := m.sessions[id]
	return runtime, ok
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

func (s *sessionRuntime) summary() Summary {
	modelName := s.record.Model
	if model, ok := llm.LookupModel(s.record.Provider, s.record.Model); ok && model.Name != "" {
		modelName = model.Name
	}
	hasApproval := false
	hasQuestion := false
	if s.transport != nil {
		hasApproval = s.transport.HasPendingApproval()
		hasQuestion = s.transport.HasPendingQuestion()
	}
	return Summary{
		ID:                  s.record.ID,
		Title:               s.displayTitle(),
		WorkspacePath:       s.record.WorkspacePath,
		WorkspaceName:       filepath.Base(s.record.WorkspacePath),
		Scope:               s.record.Scope,
		WorkspaceKind:       s.record.WorkspaceKind,
		CreatedAt:           s.record.CreatedAt,
		UpdatedAt:           s.record.UpdatedAt,
		Running:             s.live.Load(),
		HasApproval:         hasApproval,
		HasQuestion:         hasQuestion,
		ModelProvider:       s.record.Provider,
		ModelID:             s.record.Model,
		ModelName:           modelName,
		ThinkingLevel:       llm.ModelThinkingLevel(s.record.Thinking),
		PermissionMode:      permission.NormalizeMode(permission.Mode(s.record.PermissionMode)),
		ForkedFromSessionID: s.record.ForkedFromSessionID,
		ForkedFromMessageID: s.record.ForkedFromMessageID,
	}
}
