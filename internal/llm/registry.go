package llm

import (
	"errors"
	"sync"
)

// Registry stores providers by API name and is safe for concurrent access.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds or replaces the provider for its API name.
func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return errors.New("provider is nil")
	}

	api := provider.API()
	if api == "" {
		return errors.New("provider API is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[api] = provider
	return nil
}

// Get returns the provider registered for the API name.
func (r *Registry) Get(api string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[api]
	return provider, ok
}
