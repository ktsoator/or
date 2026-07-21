package web

import (
	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/llm"
)

// Session state changes are described here as plain facts — a message was
// queued, a title changed — with no knowledge of how they reach a browser.
// Turning them into SSE frames is event.go's job, and emit is the single seam
// between the two. Keeping the domain free of wire types is what will let the
// session model move out of this package later.
type sessionEvent interface{ sessionEvent() }

// messageAccepted reports a user message the server has taken responsibility
// for. Queued distinguishes one waiting behind a running turn from one the run
// has already picked up.
type messageAccepted struct {
	ID       string
	Text     string
	Images   []llm.ImageContent
	Delivery queuedDelivery
	Queued   bool
}

// messageDequeued reports a queued message the user withdrew before it ran.
type messageDequeued struct{ ID string }

// messageCancelled reports a queued message dropped because its run ended.
type messageCancelled struct{ ID string }

// titleChanged reports the session's display title and the two sources it is
// derived from, so a client can tell a user-set name from a generated one.
type titleChanged struct {
	Title       string
	AITitle     string
	CustomTitle string
}

func (messageAccepted) sessionEvent()  {}
func (messageDequeued) sessionEvent()  {}
func (messageCancelled) sessionEvent() {}
func (titleChanged) sessionEvent()     {}

// emit projects one session event and fans it out to this session's watchers.
// It is the only path from domain state to the wire, and never blocks: Hub
// drops for a slow client rather than stalling the run.
func (s *sessionRuntime) emit(event sessionEvent) {
	if data, ok := projectSessionEvent(event); ok {
		s.hub.Broadcast(data)
	}
}

// forward projects an event raised by the agent below. It is the same seam as
// emit for events the session did not originate.
func (s *sessionRuntime) forward(event coding.Event) {
	if data, ok := ProjectEvent(event); ok {
		s.hub.Broadcast(data)
	}
}
