// Package store persists a coding session's append-only transcript so it
// resumes across process restarts.
package store

import (
	"context"
	"sync"

	"github.com/ktsoator/or/coding/transcript"
)

// Store persists typed transcript entries. Compaction is an appended entry; it
// never replaces or removes original messages. A nil Store disables persistence.
type Store interface {
	Load(ctx context.Context) ([]transcript.Entry, error)
	Append(ctx context.Context, entries ...transcript.Entry) error
}

// Memory is an in-process Store useful for tests and ephemeral sessions.
type Memory struct {
	mu      sync.Mutex
	entries []transcript.Entry
}

func (m *Memory) Load(context.Context) ([]transcript.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]transcript.Entry(nil), m.entries...), nil
}

func (m *Memory) Append(_ context.Context, entries ...transcript.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entries...)
	return nil
}
