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
	mu       sync.Mutex
	entries  []Entry
	entryIDs map[string]struct{}
}

func (m *Memory) Load(context.Context) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Entry(nil), m.entries...), nil
}

func (m *Memory) Append(_ context.Context, entries ...Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entryIDs == nil && len(m.entries) > 0 {
		m.entryIDs = collectEntryIDs(m.entries)
	}
	if err := validateAppend(entries, int64(len(m.entries)), m.entryIDs); err != nil {
		return err
	}
	m.entries = append(m.entries, entries...)
	addEntryIDs(&m.entryIDs, entries)
	return nil
}
