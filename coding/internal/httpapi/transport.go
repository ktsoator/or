package httpapi

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
)

// SessionTransports owns the HTTP delivery links created for conversations.
// The conversation manager controls each link's lifetime through Transport.Close;
// handlers only look up an existing link by session ID.
type SessionTransports struct {
	mu       sync.RWMutex
	sessions map[string]*sessionTransport
	previews *previewGrantStore
}

// NewSessionTransports returns an empty session transport registry.
func NewSessionTransports() *SessionTransports {
	return &SessionTransports{
		sessions: make(map[string]*sessionTransport),
		previews: newPreviewGrantStore(),
	}
}

// New creates and registers one conversation transport.
func (r *SessionTransports) New(sessionID string) conversation.Transport {
	hub := NewHub()
	transport := &sessionTransport{
		sessionID: sessionID,
		owner:     r,
		hub:       hub,
		broker:    NewApprovalBroker(hub),
		browser:   NewBrowserBroker(hub),
	}
	r.mu.Lock()
	previous := r.sessions[sessionID]
	r.sessions[sessionID] = transport
	r.mu.Unlock()
	if previous != nil {
		r.previews.revokeSession(sessionID)
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

func (r *SessionTransports) remove(sessionID string, transport *sessionTransport) bool {
	r.mu.Lock()
	removed := false
	if r.sessions[sessionID] == transport {
		delete(r.sessions, sessionID)
		removed = true
	}
	r.mu.Unlock()
	return removed
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
	browser   *BrowserBroker
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

func (t *sessionTransport) OpenBrowser(
	ctx context.Context,
	request tools.BrowserRequest,
) (tools.BrowserResult, error) {
	if request.Preview.Path != "" {
		preview, err := t.owner.previews.issue(t.sessionID, "", request.Preview)
		if err != nil {
			return tools.BrowserResult{}, err
		}
		request.Preview = preview
	}
	result, err := t.browser.OpenBrowser(ctx, request)
	result.Preview = request.Preview
	return result, err
}

func (t *sessionTransport) Close() {
	t.closeOnce.Do(func() {
		if t.owner.remove(t.sessionID, t) {
			t.owner.previews.revokeSession(t.sessionID)
		}
		t.browser.Close()
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
			Type:     wireEventUserMessage,
			ID:       e.ID,
			Text:     e.Text,
			Images:   projectImages(e.Images),
			Delivery: projectDeliveryMode(e.Delivery),
			Queued:   e.Queued,
		}
	case conversation.MessageDequeued:
		out = wireEvent{Type: wireEventQueueRemoved, ID: e.ID}
	case conversation.MessageCancelled:
		out = wireEvent{Type: wireEventQueueCancelled, ID: e.ID}
	case conversation.RunFailed:
		out = wireEvent{Type: wireEventError, Text: e.Text}
	case conversation.TitleChanged:
		out = wireEvent{
			Type:        wireEventTitleUpdate,
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
				Type:     wireEventUserMessage,
				ID:       accepted.ID,
				Text:     accepted.Text,
				Images:   projectImages(accepted.Images),
				Delivery: projectDeliveryMode(accepted.Delivery),
				Queued:   accepted.Queued,
			})
		}
	}
	return out
}

func projectDeliveryMode(delivery conversation.Delivery) wireDeliveryMode {
	switch delivery {
	case conversation.DeliverySteer:
		return wireDeliverySteer
	case conversation.DeliveryFollowUp:
		return wireDeliveryFollowUp
	default:
		return wireDeliveryMode(delivery)
	}
}
