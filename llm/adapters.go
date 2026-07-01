package llm

import (
	"context"
	"errors"
	"sync"
)

// ProtocolAdapter translates between a concrete LLM protocol and the package streaming interface.
type ProtocolAdapter interface {
	// Protocol returns the registry key used to select this adapter.
	Protocol() Protocol

	// Stream emits response events for the given model and conversation context.
	Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error)
}

// AdapterRegistry stores protocol adapters and is safe for concurrent access.
type AdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[Protocol]ProtocolAdapter
}

// NewAdapterRegistry creates an empty protocol adapter registry.
func NewAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{
		adapters: make(map[Protocol]ProtocolAdapter),
	}
}

// Register adds or replaces an adapter for its protocol.
func (registry *AdapterRegistry) Register(adapter ProtocolAdapter) error {
	if adapter == nil {
		return errors.New("protocol adapter is nil")
	}

	protocol := adapter.Protocol()
	if protocol == "" {
		return errors.New("protocol adapter protocol is empty")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.adapters[protocol] = adapter
	return nil
}

// Get returns the adapter registered for the protocol.
func (registry *AdapterRegistry) Get(protocol Protocol) (ProtocolAdapter, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	adapter, ok := registry.adapters[protocol]
	return adapter, ok
}
