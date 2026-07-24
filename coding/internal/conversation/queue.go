package conversation

import (
	"os"
	"slices"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/llm"
)

// Messages a user sends while a run is already in flight are queued rather than
// dropped. The queue is guarded by sessionRuntime.pendingMu and is independent
// of the manager lock, so nothing here may reach for Manager.mu.

type QueuedMessage struct {
	ID       string
	Delivery Delivery
	Text     string
	Images   []llm.ImageContent
}

type pendingMessage struct {
	QueuedMessage
	Handle engine.QueueHandle
}

// QueueMessage submits one steer or follow-up to a running conversation.
func (m *Manager) QueueMessage(id string, message QueuedMessage) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrManagerClosed
	}
	runtime, ok := m.sessions[id]
	if !ok {
		m.mu.RUnlock()
		return os.ErrNotExist
	}
	if runtime.transport.HasPendingApproval() {
		m.mu.RUnlock()
		return ErrApprovalPending
	}
	if len(message.Images) > 0 {
		model, found := llm.LookupModel(runtime.record.Provider, runtime.record.Model)
		if !found || !slices.Contains(model.Input, llm.Image) {
			m.mu.RUnlock()
			return ErrImagesUnsupported
		}
	}
	m.mu.RUnlock()
	if !runtime.enqueue(message) {
		return ErrSessionNotRunning
	}
	return nil
}

func (s *sessionRuntime) enqueue(message QueuedMessage) bool {
	s.pendingMu.Lock()
	if !s.running.Load() {
		s.pendingMu.Unlock()
		return false
	}
	pending := pendingMessage{QueuedMessage: message}
	if message.Delivery == DeliverySteer {
		pending.Handle = s.session.Steer(message.Text, message.Images...)
	} else {
		pending.Handle = s.session.FollowUp(message.Text, message.Images...)
	}
	s.pending = append(s.pending, pending)
	s.pendingMu.Unlock()
	s.emit(MessageAccepted{
		ID:       message.ID,
		Text:     message.Text,
		Images:   message.Images,
		Delivery: message.Delivery,
		Queued:   true,
	})
	return true
}

// DequeueMessage withdraws one queued message by its browser-facing ID.
func (m *Manager) DequeueMessage(sessionID, messageID string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrManagerClosed
	}
	runtime, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return os.ErrNotExist
	}
	found, removed := runtime.dequeue(messageID)
	if !found {
		return ErrQueuedMessageNotFound
	}
	if !removed {
		return ErrQueuedMessageInFlight
	}
	return nil
}

func (s *sessionRuntime) dequeue(id string) (found, removed bool) {
	s.pendingMu.Lock()
	for index, message := range s.pending {
		if message.ID != id {
			continue
		}
		if !s.session.CancelQueuedMessage(message.Handle) {
			s.pendingMu.Unlock()
			return true, false
		}
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		s.pendingMu.Unlock()
		s.emit(MessageDequeued{ID: id})
		return true, true
	}
	s.pendingMu.Unlock()
	return false, false
}

func (s *sessionRuntime) consumePending(handle engine.QueueHandle) (QueuedMessage, bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for index, message := range s.pending {
		if message.Handle != handle {
			continue
		}
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		return message.QueuedMessage, true
	}
	return QueuedMessage{}, false
}

// pendingEvents replays the queue for a client that just connected.
func (s *sessionRuntime) pendingEvents() []Event {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	events := make([]Event, 0, len(s.pending))
	for _, message := range s.pending {
		events = append(events, MessageAccepted{
			ID:       message.ID,
			Text:     message.Text,
			Images:   message.Images,
			Delivery: message.Delivery,
			Queued:   true,
		})
	}
	return events
}

func (s *sessionRuntime) cancelPending() []pendingMessage {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	cancelled := append([]pendingMessage(nil), s.pending...)
	s.pending = nil
	return cancelled
}
