package httpapi

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
)

// SessionTransports owns the HTTP delivery links created for conversations.
// The conversation manager controls each link's lifetime through Transport.Close;
// handlers only look up an existing link by session ID.
type SessionTransports struct {
	mu       sync.RWMutex
	sessions map[string]*sessionTransport
}

// NewSessionTransports returns an empty session transport registry.
func NewSessionTransports() *SessionTransports {
	return &SessionTransports{sessions: make(map[string]*sessionTransport)}
}

// New creates and registers one conversation transport.
func (r *SessionTransports) New(sessionID string) conversation.Transport {
	hub := NewHub()
	transport := &sessionTransport{
		sessionID: sessionID,
		owner:     r,
		hub:       hub,
		broker:    NewApprovalBroker(hub),
	}
	r.mu.Lock()
	previous := r.sessions[sessionID]
	r.sessions[sessionID] = transport
	r.mu.Unlock()
	if previous != nil {
		previous.Close()
	}
	return transport
}

func (r *SessionTransports) get(sessionID string) (*sessionTransport, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	transport, ok := r.sessions[sessionID]
	return transport, ok
}

func (r *SessionTransports) remove(sessionID string, transport *sessionTransport) {
	r.mu.Lock()
	if r.sessions[sessionID] == transport {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()
}

// sessionTransport is this package's implementation of conversation.Transport: it
// projects what a session raises onto the SSE wire and fans it out, and it
// answers permission gates by asking the browser.
//
// The conversation package holds it as an interface while SessionTransports
// keeps the concrete delivery state on the HTTP side.
type sessionTransport struct {
	sessionID string
	owner     *SessionTransports
	hub       *Hub
	broker    *ApprovalBroker
	closeOnce sync.Once
}

func (t *sessionTransport) Publish(event conversation.Event) {
	if data, ok := projectSessionEvent(event); ok {
		t.hub.Broadcast(data)
	}
}

func (t *sessionTransport) PublishAgent(event engine.Event) {
	if data, ok := ProjectEvent(event); ok {
		t.hub.Broadcast(data)
	}
}

func (t *sessionTransport) Decide(ctx context.Context, req permission.ApprovalRequest) (permission.ApprovalResponse, error) {
	return t.broker.Decide(ctx, req)
}

func (t *sessionTransport) HasPendingApproval() bool {
	return t.broker.HasPending()
}

func (t *sessionTransport) Close() {
	t.closeOnce.Do(func() {
		t.owner.remove(t.sessionID, t)
		t.hub.Close()
	})
}

// projectSessionEvent maps a session state change to the HTTP wire protocol.
// It is the counterpart to ProjectEvent: that one projects events coming up
// from the agent, this one projects events the session layer raises itself.
func projectSessionEvent(event conversation.Event) ([]byte, bool) {
	var out wireEvent
	switch e := event.(type) {
	case conversation.MessageAccepted:
		out = wireEvent{
			Type:     "user_message",
			ID:       e.ID,
			Text:     e.Text,
			Images:   projectImages(e.Images),
			Delivery: string(e.Delivery),
			Queued:   e.Queued,
		}
	case conversation.MessageDequeued:
		out = wireEvent{Type: "queue_removed", ID: e.ID}
	case conversation.MessageCancelled:
		out = wireEvent{Type: "queue_cancelled", ID: e.ID}
	case conversation.RunFailed:
		out = wireEvent{Type: "error", Text: e.Text}
	case conversation.TitleChanged:
		out = wireEvent{
			Type:        "title_update",
			Title:       e.Title,
			AITitle:     e.AITitle,
			CustomTitle: e.CustomTitle,
		}
	default:
		return nil, false
	}
	data, err := json.Marshal(out)
	return data, err == nil
}

// projectQueue maps the queue snapshot the history endpoint returns.
func projectQueue(events []conversation.Event) []wireEvent {
	out := make([]wireEvent, 0, len(events))
	for _, event := range events {
		if accepted, ok := event.(conversation.MessageAccepted); ok {
			out = append(out, wireEvent{
				Type:     "user_message",
				ID:       accepted.ID,
				Text:     accepted.Text,
				Images:   projectImages(accepted.Images),
				Delivery: string(accepted.Delivery),
				Queued:   accepted.Queued,
			})
		}
	}
	return out
}
