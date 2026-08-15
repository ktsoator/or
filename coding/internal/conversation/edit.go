package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

// EditMessage rewrites one idle conversation at a historical user message and
// continues from the replacement without changing the session or workspace.
func (m *Manager) EditMessage(id, messageID, replacementText string) (Summary, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Summary{}, ErrManagerClosed
	}
	runtime, err := m.loadRuntimeLocked(id)
	if err != nil {
		m.mu.Unlock()
		return Summary{}, err
	}
	if runtime.running.Load() || runtime.live.Load() || runtime.awaitingUser() ||
		runtime.titleGenerating.Load() || runtime.hasRunningTask() {
		m.mu.Unlock()
		return Summary{}, ErrSessionActive
	}

	originalEntries := runtime.session.Entries()
	rewritten, err := transcript.Fork(
		originalEntries,
		strings.TrimSpace(messageID),
		transcript.ForkBeforeUser,
		replacementText,
	)
	if err != nil {
		m.mu.Unlock()
		return Summary{}, err
	}
	store := transcript.NewJSONL(runtime.record.Transcript)
	if err := store.Replace(m.ctx, rewritten); err != nil {
		m.mu.Unlock()
		return Summary{}, fmt.Errorf("session: replace transcript: %w", err)
	}
	replacement, err := m.rebuildSession(runtime)
	if err != nil {
		restoreErr := store.Replace(m.ctx, originalEntries)
		m.mu.Unlock()
		return Summary{}, errors.Join(
			fmt.Errorf("session: rebuild edited session: %w", err),
			restoreTranscriptError(restoreErr),
		)
	}

	previousRecord := runtime.record
	invalidatedBranches := m.invalidateDiscardedBranchPointsLocked(id, rewritten)
	runtime.record.UpdatedAt = time.Now().UTC()
	if runtime.record.UsageBackfillOffset > len(rewritten) {
		runtime.record.UsageBackfillOffset = len(rewritten)
	}
	if err := m.saveLocked(); err != nil {
		runtime.record = previousRecord
		for child, record := range invalidatedBranches {
			child.record = record
		}
		replacement.Close()
		restoreErr := store.Replace(m.ctx, originalEntries)
		m.mu.Unlock()
		return Summary{}, errors.Join(err, restoreTranscriptError(restoreErr))
	}

	previousSession := runtime.session
	runtime.session = replacement
	runtime.running.Store(true)
	runtime.live.Store(true)
	m.tasks.Add(1)
	summary := runtime.summary()
	m.mu.Unlock()

	previousSession.Close()
	runtime.emit(HistoryRewritten{})
	go m.continueEditedSession(id, runtime)
	return summary, nil
}

func (m *Manager) rebuildSession(runtime *sessionRuntime) (*engine.Session, error) {
	model, _ := llm.LookupModel(runtime.record.Provider, runtime.record.Model)
	additionalTools := runtime.mcpLease.Tools()
	session, err := newEngineSession(m.ctx, engineSessionConfig{
		SessionID:       runtime.record.ID,
		WorkspacePath:   runtime.record.WorkspacePath,
		TranscriptPath:  runtime.record.Transcript,
		Model:           model,
		ThinkingLevel:   llm.ModelThinkingLevel(runtime.record.Thinking),
		PermissionMode:  permission.Mode(runtime.record.PermissionMode),
		AdditionalTools: additionalTools,
		StreamFn:        m.streamFn,
		Recorder:        m.recorder,
	}, runtime.transport)
	if err != nil {
		return nil, err
	}
	session.Subscribe(func(event engine.Event) {
		m.handleSessionEvent(runtime.record.ID, runtime, event)
	})
	return session, nil
}

func (m *Manager) continueEditedSession(id string, runtime *sessionRuntime) {
	defer m.finishRun(id, runtime)
	if err := runtime.session.Continue(m.ctx); err != nil &&
		!errors.Is(err, context.Canceled) {
		runtime.emit(RunFailed{Text: err.Error()})
	}
}

func (m *Manager) invalidateDiscardedBranchPointsLocked(
	parentID string,
	entries []transcript.Entry,
) map[*sessionRuntime]record {
	retained := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		retained[entry.ID] = struct{}{}
	}
	invalidated := make(map[*sessionRuntime]record)
	for _, child := range m.sessions {
		if child.record.ForkedFromSessionID != parentID ||
			child.record.ForkedFromMessageID == "" {
			continue
		}
		if _, ok := retained[child.record.ForkedFromMessageID]; ok {
			continue
		}
		invalidated[child] = child.record
		child.record.ForkedFromMessageID = ""
	}
	return invalidated
}

func restoreTranscriptError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("session: restore transcript after failed edit: %w", err)
}
