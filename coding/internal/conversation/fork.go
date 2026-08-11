package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/transcript"
)

// ForkOptions selects a durable message boundary and, for user-message edits,
// carries the replacement text used to start the child conversation.
type ForkOptions struct {
	MessageID       string
	Mode            transcript.ForkMode
	ReplacementText string
}

// Fork creates a new conversation from one idle source session. Both sessions
// share the current workspace, while their transcripts and future model runs
// remain independent. Editing a user message starts the child response after
// the new record is durable; assistant-response forks remain idle.
func (m *Manager) Fork(sourceID string, options ForkOptions) (Summary, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Summary{}, ErrManagerClosed
	}
	source, err := m.loadRuntimeLocked(sourceID)
	if err != nil {
		m.mu.Unlock()
		return Summary{}, err
	}
	if source.running.Load() || source.live.Load() || source.awaitingUser() ||
		source.titleGenerating.Load() || source.hasRunningTask() {
		m.mu.Unlock()
		return Summary{}, ErrSessionActive
	}

	messageID := strings.TrimSpace(options.MessageID)
	entries, err := transcript.Fork(
		source.session.Entries(),
		messageID,
		options.Mode,
		options.ReplacementText,
	)
	if err != nil {
		m.mu.Unlock()
		return Summary{}, err
	}

	now := time.Now().UTC()
	id := NewID()
	for m.sessions[id] != nil {
		id = NewID()
	}
	record := record{
		ID:                  id,
		Title:               source.record.Title,
		CustomTitle:         source.record.CustomTitle,
		WorkspacePath:       source.record.WorkspacePath,
		Scope:               source.record.Scope,
		WorkspaceKind:       source.record.WorkspaceKind,
		CreatedAt:           now,
		UpdatedAt:           now,
		Transcript:          filepath.Join(filepath.Dir(m.indexPath), id+".jsonl"),
		Provider:            source.record.Provider,
		Model:               source.record.Model,
		Thinking:            source.record.Thinking,
		PermissionMode:      source.record.PermissionMode,
		ForkedFromSessionID: sourceID,
		ForkedFromMessageID: messageID,
		UsageBackfillOffset: len(entries),
	}
	store := transcript.NewJSONL(record.Transcript)
	if err := store.Append(m.ctx, entries...); err != nil {
		m.mu.Unlock()
		return Summary{}, fmt.Errorf("session: persist fork transcript: %w", err)
	}

	child, err := m.build(record)
	if err != nil {
		_ = os.Remove(record.Transcript)
		m.mu.Unlock()
		return Summary{}, fmt.Errorf("session: build fork: %w", err)
	}
	continueRun := options.Mode == transcript.ForkBeforeUser
	if continueRun {
		child.running.Store(true)
		child.live.Store(true)
	}
	m.sessions[id] = child
	if err := m.saveLocked(); err != nil {
		delete(m.sessions, id)
		child.close()
		_ = os.Remove(record.Transcript)
		m.mu.Unlock()
		return Summary{}, err
	}
	if continueRun {
		m.tasks.Add(1)
	}
	summary := child.summary()
	m.mu.Unlock()

	if continueRun {
		go m.continueFork(id, child)
	}
	return summary, nil
}

func (m *Manager) continueFork(id string, runtime *sessionRuntime) {
	defer m.finishRun(id, runtime)
	if err := runtime.session.Continue(m.ctx); err != nil &&
		!errors.Is(err, context.Canceled) {
		runtime.emit(RunFailed{Text: err.Error()})
	}
}
