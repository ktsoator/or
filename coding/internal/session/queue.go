package session

import (
	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/llm"
)

// Messages a user sends while a run is already in flight are queued rather than
// dropped. The queue is guarded by Runtime.pendingMu and is independent
// of the manager lock, so nothing here may reach for Manager.mu.

type QueuedMessage struct {
	ID       string
	Delivery Delivery
	Text     string
	Images   []llm.ImageContent
	Handle   coding.QueueHandle
}

func (s *Runtime) Queue(message QueuedMessage) bool {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if !s.running.Load() {
		return false
	}
	if message.Delivery == DeliverySteer {
		message.Handle = s.session.Steer(message.Text, message.Images...)
	} else {
		message.Handle = s.session.FollowUp(message.Text, message.Images...)
	}
	s.pending = append(s.pending, message)
	s.emit(MessageAccepted{
		ID:       message.ID,
		Text:     message.Text,
		Images:   message.Images,
		Delivery: message.Delivery,
		Queued:   true,
	})
	return true
}

func (s *Runtime) Dequeue(id string) (found, removed bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for index, message := range s.pending {
		if message.ID != id {
			continue
		}
		if !s.session.CancelQueuedMessage(message.Handle) {
			return true, false
		}
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		s.emit(MessageDequeued{ID: id})
		return true, true
	}
	return false, false
}

func (s *Runtime) consumePending(text string, images []llm.ImageContent) (QueuedMessage, bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for index, message := range s.pending {
		if message.Text != text || !sameImages(message.Images, images) {
			continue
		}
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		return message, true
	}
	return QueuedMessage{}, false
}

// pendingEvents replays the queue for a client that just connected.
func (s *Runtime) PendingEvents() []Event {
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

func (s *Runtime) cancelPending() []QueuedMessage {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	cancelled := append([]QueuedMessage(nil), s.pending...)
	s.pending = nil
	return cancelled
}

func sameImages(left, right []llm.ImageContent) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].MIMEType != right[index].MIMEType || left[index].Data != right[index].Data {
			return false
		}
	}
	return true
}
