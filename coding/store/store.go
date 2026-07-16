// Package store persists a coding session's transcript so it resumes across
// process restarts. A Store is message-shaped: it loads, appends, and (for
// compaction) replaces a flat list of agent messages. Richer models — typed
// entries, branching, checkpoints — are a later concern layered above this.
package store

import (
	"context"
	"sync"

	"github.com/ktsoator/or/agent"
)

// Store persists a transcript. Load returns the prior transcript to seed a
// session; Append records the messages a run added, in order; Replace overwrites
// the whole transcript, used when compaction rewrites history. A nil Store means
// no persistence.
type Store interface {
	Load(ctx context.Context) ([]agent.AgentMessage, error)
	Append(ctx context.Context, messages ...agent.AgentMessage) error
	Replace(ctx context.Context, messages []agent.AgentMessage) error
}

// Memory is an in-process Store backed by a slice. It persists only for the
// lifetime of the value, which makes it a useful default for tests and ephemeral
// sessions. It is safe for concurrent use.
type Memory struct {
	mu       sync.Mutex
	messages []agent.AgentMessage
}

// Load returns a copy of the retained transcript.
func (m *Memory) Load(context.Context) ([]agent.AgentMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agent.AgentMessage(nil), m.messages...), nil
}

// Append retains a copy of the given messages.
func (m *Memory) Append(_ context.Context, messages ...agent.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, messages...)
	return nil
}

// Replace overwrites the retained transcript.
func (m *Memory) Replace(_ context.Context, messages []agent.AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append([]agent.AgentMessage(nil), messages...)
	return nil
}
