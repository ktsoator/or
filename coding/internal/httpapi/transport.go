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
		questions: NewQuestionBroker(hub),
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
	questions *QuestionBroker
	activeRun activeRunHistory
	closeOnce sync.Once
}

func (t *sessionTransport) Publish(event conversation.Event) {
	if projected, ok := projectSessionWireEvent(event); ok {
		t.publish(projected)
	}
}

func (t *sessionTransport) PublishAgent(event engine.Event) {
	if projected, ok := projectEvent(event); ok {
		t.publish(projected)
	}
}

func (t *sessionTransport) publish(event wireEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	t.hub.broadcast(data, func() { t.activeRun.apply(event) })
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

func (t *sessionTransport) InspectBrowser(ctx context.Context, tabID string) (tools.BrowserInspectionResult, error) {
	return t.browser.InspectBrowser(ctx, tabID)
}

func (t *sessionTransport) BrowserTabs(ctx context.Context) (tools.BrowserTabsResult, error) {
	return t.browser.BrowserTabs(ctx)
}

func (t *sessionTransport) Ask(
	ctx context.Context,
	questions []tools.Question,
) ([]tools.Answer, error) {
	return t.questions.Ask(ctx, questions)
}

func (t *sessionTransport) HasPendingQuestion() bool {
	return t.questions.HasPending()
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

// TryCloseIfIdle lets the conversation manager retire this transport without
// racing a newly attached SSE viewer.
func (t *sessionTransport) TryCloseIfIdle() bool {
	if !t.hub.closeIfIdle() {
		return false
	}
	t.Close()
	return true
}

// projectSessionEvent maps a session state change to the HTTP wire protocol.
// It is the counterpart to ProjectEvent: that one projects events coming up
// from the agent, this one projects events the session layer raises itself.
func projectSessionEvent(event conversation.Event) ([]byte, bool) {
	out, ok := projectSessionWireEvent(event)
	if !ok {
		return nil, false
	}
	data, err := json.Marshal(out)
	return data, err == nil
}

func projectSessionWireEvent(event conversation.Event) (wireEvent, bool) {
	var out wireEvent
	switch e := event.(type) {
	case conversation.MessageAccepted:
		out = wireEvent{
			Type:     wireEventUserMessage,
			ID:       e.ID,
			Text:     e.Text,
			Images:   projectImages(e.Images),
			Files:    projectFiles(e.Files),
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
			Type:  wireEventTitleUpdate,
			Title: e.Title,
		}
	default:
		return wireEvent{}, false
	}
	return out, true
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
				Files:    projectFiles(accepted.Files),
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
