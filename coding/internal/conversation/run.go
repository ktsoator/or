package conversation

import (
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/llm"
)

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
		return nil, engine.ErrBusy
	}
	runtime.live.Store(true)
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
		runtime.live.Store(false)
		return nil, err
	}
	// Broadcast title change so the client updates immediately.
	runtime.broadcastTitle()
	return runtime, nil
}

// BeginCompact reserves an idle session for a manual context compaction. It
// does not alter the visible title or transcript; engine commits the compaction
// boundary only after summary generation succeeds.
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
	runtime.live.Store(true)
	previous := runtime.record.UpdatedAt
	runtime.record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(); err != nil {
		runtime.record.UpdatedAt = previous
		runtime.running.Store(false)
		runtime.live.Store(false)
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
	runtime.live.Store(false)
	runtime.record.UpdatedAt = time.Now().UTC()
	_ = m.saveLocked()
	m.mu.Unlock()

	cancelled := runtime.cancelPending()
	runtime.session.ClearQueuedMessages()
	for _, message := range cancelled {
		runtime.emit(MessageCancelled{ID: message.ID})
	}
}
