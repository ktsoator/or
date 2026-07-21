package api

import (
	"encoding/json"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/session"
	"github.com/ktsoator/or/coding/policy"
)

// sessionTransport is this package's implementation of session.Transport: it
// projects what a session raises onto the SSE wire and fans it out, and it
// answers permission gates by asking the browser.
//
// The session package holds it as an interface, so a handler that needs the
// SSE plumbing asserts back to this type. That is safe because this package is
// the only thing that ever builds one, and it keeps the session model from
// having to describe channels or byte frames it does not care about.
type sessionTransport struct {
	hub    *Hub
	broker *ConfirmBroker
}

func newSessionTransport() *sessionTransport {
	hub := NewHub()
	return &sessionTransport{hub: hub, broker: NewConfirmBroker(hub)}
}

func (t *sessionTransport) Publish(event session.Event) {
	if data, ok := projectSessionEvent(event); ok {
		t.hub.Broadcast(data)
	}
}

func (t *sessionTransport) PublishAgent(event coding.Event) {
	if data, ok := ProjectEvent(event); ok {
		t.hub.Broadcast(data)
	}
}

func (t *sessionTransport) Confirm(req policy.Request) bool {
	return t.broker.Confirm(req)
}

func (t *sessionTransport) HasPendingApproval() bool {
	return t.broker.HasPending()
}

// send delivers an already-projected frame, for the API layer's own messages.
func (t *sessionTransport) send(data []byte) { t.hub.Broadcast(data) }

// transportOf returns the delivery link this package attached to a session.
func transportOf(runtime *session.Runtime) *sessionTransport {
	return runtime.Transport().(*sessionTransport)
}

// projectSessionEvent maps a session state change to the HTTP wire protocol.
// It is the counterpart to ProjectEvent: that one projects events coming up
// from the agent, this one projects events the session layer raises itself.
func projectSessionEvent(event session.Event) ([]byte, bool) {
	var out wireEvent
	switch e := event.(type) {
	case session.MessageAccepted:
		out = wireEvent{
			Type:     "user_message",
			ID:       e.ID,
			Text:     e.Text,
			Images:   projectImages(e.Images),
			Delivery: string(e.Delivery),
			Queued:   e.Queued,
		}
	case session.MessageDequeued:
		out = wireEvent{Type: "queue_removed", ID: e.ID}
	case session.MessageCancelled:
		out = wireEvent{Type: "queue_cancelled", ID: e.ID}
	case session.TitleChanged:
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
func projectQueue(events []session.Event) []wireEvent {
	out := make([]wireEvent, 0, len(events))
	for _, event := range events {
		if accepted, ok := event.(session.MessageAccepted); ok {
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
