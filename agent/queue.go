package agent

import "sync"

// QueueMode controls how many queued steering or follow-up messages are injected
// at one drain point.
type QueueMode string

const (
	// QueueAll injects every queued message at the drain point.
	QueueAll QueueMode = "all"
	// QueueOneAtATime injects only the oldest queued message, leaving the rest for
	// later drain points. It is the default.
	QueueOneAtATime QueueMode = "one-at-a-time"
)

// QueueHandle is an opaque identity for one message submitted to an Agent
// queue. It remains attached to that message when drained, while cancellation
// succeeds only until the message leaves the queue.
type QueueHandle struct {
	queue *messageQueue
	id    uint64
}

// Steer queues a message to inject into the current run before its next turn.
func (a *Agent) Steer(message AgentMessage) QueueHandle {
	return QueueHandle{queue: a.steering, id: a.steering.enqueue(message)}
}

// FollowUp queues a message to process after the current run would stop.
func (a *Agent) FollowUp(message AgentMessage) QueueHandle {
	return QueueHandle{queue: a.followUp, id: a.followUp.enqueue(message)}
}

// CancelQueued removes one message if it has not already been drained for
// processing. Handles from another Agent are rejected.
func (a *Agent) CancelQueued(handle QueueHandle) bool {
	if handle.queue == nil || (handle.queue != a.steering && handle.queue != a.followUp) {
		return false
	}
	return handle.queue.remove(handle.id)
}

// HasQueuedMessages reports whether any steering or follow-up message is queued.
func (a *Agent) HasQueuedMessages() bool {
	return a.steering.hasItems() || a.followUp.hasItems()
}

// ClearSteeringQueue drops all queued steering messages.
func (a *Agent) ClearSteeringQueue() { a.steering.clear() }

// ClearFollowUpQueue drops all queued follow-up messages.
func (a *Agent) ClearFollowUpQueue() { a.followUp.clear() }

// ClearQueues drops all queued steering and follow-up messages.
func (a *Agent) ClearQueues() {
	a.steering.clear()
	a.followUp.clear()
}

// queueModeOrDefault resolves an unset QueueMode to the default, QueueOneAtATime.
func queueModeOrDefault(mode QueueMode) QueueMode {
	if mode == "" {
		return QueueOneAtATime
	}
	return mode
}

// messageQueue is a concurrency-safe FIFO backing the steering and follow-up
// queues. Its mode decides how many messages one drain returns.
type messageQueue struct {
	mu     sync.Mutex
	mode   QueueMode
	nextID uint64
	items  []queuedMessage
}

type queuedMessage struct {
	id      uint64
	message AgentMessage
}

func (q *messageQueue) enqueue(message AgentMessage) uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.nextID++
	q.items = append(q.items, queuedMessage{id: q.nextID, message: message})
	return q.nextID
}

func (q *messageQueue) clear() {
	q.mu.Lock()
	q.items = nil
	q.mu.Unlock()
}

func (q *messageQueue) hasItems() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) > 0
}

func (q *messageQueue) remove(id uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for index, item := range q.items {
		if item.id != id {
			continue
		}
		q.items = append(q.items[:index], q.items[index+1:]...)
		return true
	}
	return false
}

// drain returns queued messages: the oldest one when the mode is
// QueueOneAtATime, otherwise all of them.
func (q *messageQueue) drain() []AgentMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	if q.mode == QueueOneAtATime {
		next := q.items[0]
		q.items = append([]queuedMessage(nil), q.items[1:]...)
		return []AgentMessage{queuedMessageEnvelope{
			message: next.message,
			handle:  QueueHandle{queue: q, id: next.id},
		}}
	}
	drained := q.items
	q.items = nil
	messages := make([]AgentMessage, 0, len(drained))
	for _, item := range drained {
		messages = append(messages, queuedMessageEnvelope{
			message: item.message,
			handle:  QueueHandle{queue: q, id: item.id},
		})
	}
	return messages
}
