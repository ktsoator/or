package conversation

import (
	"context"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

// Session state changes are described here as plain facts — a message was
// queued, a title changed — with no knowledge of how they reach a viewer.
// Projecting them onto a wire format belongs to whoever is delivering them.
type Event interface{ Event() }

// Transport is one session's link to whatever is watching it. The delivery
// layer supplies an implementation per session; this package never learns how
// an event is encoded or who receives it.
//
// Decide and HasPendingApproval are here rather than on a separate type
// because a permission gate is a conversation with the same viewer: the
// session cannot know whether one is answerable without asking its transport.
type Transport interface {
	// Publish delivers a state change this session raised.
	Publish(Event)
	// PublishAgent delivers an event raised by the agent underneath.
	PublishAgent(engine.Event)
	// Decide gates one tool call, blocking until answered or cancelled.
	Decide(context.Context, permission.ApprovalRequest) (permission.ApprovalResponse, error)
	// HasPendingApproval reports a gate still waiting on a viewer.
	HasPendingApproval() bool
}

// NewTransport builds the delivery link for one session. Manager calls it once
// per session, at construction, and hands back the result via Runtime.Transport.
type NewTransport func(sessionID string) Transport

// MessageAccepted reports a user message the server has taken responsibility
// for. Queued distinguishes one waiting behind a running turn from one the run
// has already picked up.
type MessageAccepted struct {
	ID       string
	Text     string
	Images   []llm.ImageContent
	Delivery Delivery
	Queued   bool
}

// MessageDequeued reports a queued message the user withdrew before it ran.
type MessageDequeued struct{ ID string }

// MessageCancelled reports a queued message dropped because its run ended.
type MessageCancelled struct{ ID string }

// TitleChanged reports the session's display title and the two sources it is
// derived from, so a client can tell a user-set name from a generated one.
type TitleChanged struct {
	Title       string
	AITitle     string
	CustomTitle string
}

func (MessageAccepted) Event()  {}
func (MessageDequeued) Event()  {}
func (MessageCancelled) Event() {}
func (TitleChanged) Event()     {}

// Transport returns this session's delivery link, for a caller that needs to
// reach the concrete implementation it supplied.
func (s *Runtime) Transport() Transport { return s.transport }

// emit hands one state change to the transport. It must not block: a session
// raising an event is often mid-run.
func (s *Runtime) emit(event Event) { s.transport.Publish(event) }

// forward hands on an event raised by the agent below.
func (s *Runtime) forward(event engine.Event) { s.transport.PublishAgent(event) }
