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
	Running      bool
	Title        string
	AITitle      string
	CustomTitle  string
}

// Snapshot returns the current client-readable state without exposing the
// runtime that owns the engine session.
func (m *Manager) Snapshot(id string) (Snapshot, error) {
	m.mu.RLock()
	runtime, ok := m.sessions[id]
	var title TitleChanged
	if ok {
		title = runtime.titleChanged()
	}
	m.mu.RUnlock()
	if !ok {
		return Snapshot{}, os.ErrNotExist
	}
	return Snapshot{
		History:      runtime.session.History(),
		Queue:        runtime.pendingEvents(),
		ContextUsage: runtime.session.ContextUsage(),
		Running:      runtime.live.Load(),
		Title:        title.Title,
		AITitle:      title.AITitle,
		CustomTitle:  title.CustomTitle,
	}, nil
}

// WorkspacePath returns the tool root owned by one conversation.
func (m *Manager) WorkspacePath(id string) (string, error) {
	m.mu.RLock()
	runtime, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return "", os.ErrNotExist
	}
	return runtime.session.Cwd(), nil
}

// Abort cancels the active run, if any.
func (m *Manager) Abort(id string) error {
	m.mu.RLock()
	runtime, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return os.ErrNotExist
	}
	runtime.session.Abort()
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

// UsesProvider reports whether any restored session currently references the
// provider.
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

func (s *sessionRuntime) summary() Summary {
	modelName := s.record.Model
	if model, ok := llm.LookupModel(s.record.Provider, s.record.Model); ok && model.Name != "" {
		modelName = model.Name
	}
	return Summary{
		ID:             s.record.ID,
		Title:          s.displayTitle(),
		AITitle:        s.record.AITitle,
		CustomTitle:    s.record.CustomTitle,
		WorkspacePath:  s.record.WorkspacePath,
		WorkspaceName:  filepath.Base(s.record.WorkspacePath),
		Scope:          s.record.Scope,
		WorkspaceKind:  s.record.WorkspaceKind,
		CreatedAt:      s.record.CreatedAt,
		UpdatedAt:      s.record.UpdatedAt,
		Running:        s.live.Load(),
		HasApproval:    s.transport.HasPendingApproval(),
		ModelProvider:  s.record.Provider,
		ModelID:        s.record.Model,
		ModelName:      modelName,
		ThinkingLevel:  llm.ModelThinkingLevel(s.record.Thinking),
		PermissionMode: permission.NormalizeMode(permission.Mode(s.record.PermissionMode)),
	}
}
