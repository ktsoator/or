package conversation

import (
	"path/filepath"
	"slices"
	"sort"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

func (m *Manager) Get(id string) (*Runtime, bool) {
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

func (s *Runtime) summary() Summary {
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

// History returns the displayable transcript without exposing the engine
// session that owns it.
func (s *Runtime) History() []engine.HistoryItem { return s.session.History() }

// ContextUsage returns the current context-window estimate.
func (s *Runtime) ContextUsage() engine.ContextUsage { return s.session.ContextUsage() }

// WorkspacePath returns the root used by this conversation's tools.
func (s *Runtime) WorkspacePath() string { return s.session.Cwd() }

// SupportsImages reports whether the active model accepts image input.
func (s *Runtime) SupportsImages() bool {
	return slices.Contains(s.session.Snapshot().Model.Input, llm.Image)
}

// Abort cancels the active run, if any.
func (s *Runtime) Abort() { s.session.Abort() }

// Running reports the live state exposed to clients. It clears before the
// terminal event is published, while the internal reservation remains held
// until the manager finishes its cleanup.
func (s *Runtime) Running() bool { return s.live.Load() }

// HasPendingApproval reports a permission gate still waiting on an answer.
func (s *Runtime) HasPendingApproval() bool { return s.transport.HasPendingApproval() }
