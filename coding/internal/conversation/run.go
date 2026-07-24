package conversation

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/llm"
)

// StartPrompt reserves a session and runs the prompt in the background. The
// manager owns the complete lifecycle so callers cannot forget to release the
// reservation or clean up queued messages.
func (m *Manager) StartPrompt(id, prompt string, images ...llm.ImageContent) error {
	runtime, err := m.reservePrompt(id, prompt, images)
	if err != nil {
		return err
	}
	images = slices.Clone(images)
	go func() {
		defer m.finishRun(id, runtime)
		if err := runtime.session.Prompt(m.ctx, prompt, images...); err != nil &&
			!errors.Is(err, context.Canceled) {
			runtime.emit(RunFailed{Text: err.Error()})
		}
	}()
	return nil
}

// Compact reserves an idle session and performs one explicit context
// compaction. It blocks until the summary is durable.
func (m *Manager) Compact(
	ctx context.Context,
	id string,
	instructions string,
) (engine.CompactionResult, error) {
	runtime, err := m.reserveCompact(id)
	if err != nil {
		return engine.CompactionResult{}, err
	}
	defer m.finishRun(id, runtime)
	return runtime.session.Compact(ctx, instructions)
}

func (m *Manager) reservePrompt(
	id string,
	prompt string,
	images []llm.ImageContent,
) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
	runtime, ok := m.sessions[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	if len(images) > 0 {
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
		if strings.TrimSpace(title) == "" && len(images) > 0 {
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
	m.tasks.Add(1)
	// Broadcast title change so the client updates immediately.
	runtime.broadcastTitle()
	return runtime, nil
}

func (m *Manager) reserveCompact(id string) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
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
	m.tasks.Add(1)
	return runtime, nil
}

// finishRun clears live activity, records when the session last finished, and
// drops queued messages that the run did not consume.
func (m *Manager) finishRun(id string, runtime *Runtime) {
	m.mu.Lock()
	if current, ok := m.sessions[id]; ok && current == runtime {
		runtime.running.Store(false)
		runtime.live.Store(false)
		runtime.record.UpdatedAt = time.Now().UTC()
		_ = m.saveLocked()
	}
	m.mu.Unlock()

	cancelled := runtime.cancelPending()
	runtime.session.ClearQueuedMessages()
	for _, message := range cancelled {
		runtime.emit(MessageCancelled{ID: message.ID})
	}
	m.tasks.Done()
}
