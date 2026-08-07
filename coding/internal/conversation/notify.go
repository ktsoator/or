package conversation

import (
	"context"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/invocation"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
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
	// OpenBrowser delivers one validated agent navigation and waits for the
	// viewer's product shell to report its terminal result.
	OpenBrowser(context.Context, tools.BrowserRequest) (tools.BrowserResult, error)
	// Ask puts the agent's multiple-choice questions to the viewer and blocks
	// until they answer or the run is cancelled.
	Ask(context.Context, []tools.Question) ([]tools.Answer, error)
	// HasPendingQuestion reports a question still waiting on a viewer.
	HasPendingQuestion() bool
	// Close releases the delivery link after its session is removed.
	Close()
}

// NewTransport builds the delivery link for one opened session. Manager calls
// it once when the session runtime loads and owns closing the returned transport.
type NewTransport func(sessionID string) Transport

// idleClosingTransport is implemented by delivery layers that can atomically
// prove no viewer is attached while retiring their replay state. Keeping this
// capability optional lets non-HTTP adapters pin their runtime until shutdown.
type idleClosingTransport interface {
	TryCloseIfIdle() bool
}

// MessageAccepted reports a user message the server has taken responsibility
// for. Queued distinguishes one waiting behind a running turn from one the run
// has already picked up.
type MessageAccepted struct {
	ID         string
	Text       string
	Images     []llm.ImageContent
	Files      []engine.File
	Invocation *invocation.Record
	Delivery   Delivery
	Queued     bool
}

// MessageDequeued reports a queued message the user withdrew before it ran.
type MessageDequeued struct{ ID string }

// MessageCancelled reports a queued message dropped because its run ended.
type MessageCancelled struct{ ID string }

// RunFailed reports an asynchronous prompt failure to the viewer.
type RunFailed struct{ Text string }

// TitleChanged reports the session's display title and the two sources it is
// derived from, so a client can tell a user-set name from a generated one.
type TitleChanged struct {
	Title       string
	AITitle     string
	CustomTitle string
}

type TitleGenerationChanged struct{ Generation TitleGeneration }

func (MessageAccepted) Event()        {}
func (MessageDequeued) Event()        {}
func (MessageCancelled) Event()       {}
func (RunFailed) Event()              {}
func (TitleChanged) Event()           {}
func (TitleGenerationChanged) Event() {}

// emit hands one state change to the transport. It must not block: a session
// raising an event is often mid-run.
func (s *sessionRuntime) emit(event Event) {
	if s.transport != nil {
		s.transport.Publish(event)
	}
}

// forward hands on an event raised by the agent below.
func (s *sessionRuntime) forward(event engine.Event) {
	if s.transport != nil {
		s.transport.PublishAgent(event)
	}
}
