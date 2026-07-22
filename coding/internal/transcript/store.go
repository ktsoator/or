package transcript

import (
	"context"
	"sync"
)

// Store persists typed transcript entries. Compaction is an appended entry; it
// never replaces or removes original messages. A nil Store disables persistence.
type Store interface {
	Load(ctx context.Context) ([]Entry, error)
	Append(ctx context.Context, entries ...Entry) error
}

// Memory is an in-process Store useful for tests and ephemeral sessions.
type Memory struct {
	mu      sync.Mutex
	entries []Entry
}

func (m *Memory) Load(context.Context) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Entry(nil), m.entries...), nil
}

func (m *Memory) Append(_ context.Context, entries ...Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entries...)
	return nil
}
