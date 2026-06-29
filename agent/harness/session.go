package harness

import (
	"context"
	"sync"

	"github.com/ktsoator/or/agent"
)

// Session persists an agent transcript across runs. Load returns the prior
// transcript a Harness seeds itself with at construction; Append receives the
// messages each run adds, in transcript order, after the run completes.
//
// Implementations should treat the slices as read-only and copy anything they
// retain. A nil Session disables persistence.
type Session interface {
	// Load returns the persisted transcript, oldest message first. It returns an
	// empty slice (not an error) for a session that has no history yet.
	Load(ctx context.Context) ([]agent.AgentMessage, error)
	// Append records the messages a run added to the transcript, in order.
	Append(ctx context.Context, messages ...agent.AgentMessage) error
}

// ReplaceableSession is a Session that can overwrite its whole transcript. The
// harness requires it for Compact, which rewrites history to a compacted form;
// a plain Session is append-only and cannot support that.
type ReplaceableSession interface {
	Session
	// Replace overwrites the stored transcript with messages, in order.
	Replace(ctx context.Context, messages []agent.AgentMessage) error
}

// InMemorySession is a Session backed by an in-process slice. It persists for
// the lifetime of the value only, which makes it a useful default for tests and
// ephemeral sessions. It is safe for concurrent use.
type InMemorySession struct {
	mu       sync.Mutex
	messages []agent.AgentMessage
}

// Load returns a copy of the retained transcript.
func (s *InMemorySession) Load(context.Context) ([]agent.AgentMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]agent.AgentMessage(nil), s.messages...), nil
}

// Append retains a copy of the given messages.
func (s *InMemorySession) Append(_ context.Context, messages ...agent.AgentMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, messages...)
	return nil
}

// Replace overwrites the retained transcript.
func (s *InMemorySession) Replace(_ context.Context, messages []agent.AgentMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append([]agent.AgentMessage(nil), messages...)
	return nil
}
