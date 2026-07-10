package llm

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// ProviderRegistry stores provider runtimes and their overrides, and resolves
// requests through them. It is safe for concurrent access.
//
// It complements the ModelRegistry: the ModelRegistry answers "which models
// exist under this provider", the ProviderRegistry answers "how is this
// provider configured" and applies that configuration to outgoing requests.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]*Provider
	overrides map[string]ProviderOverride
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]*Provider),
		overrides: make(map[string]ProviderOverride),
	}
}

// NewBuiltInProviderRegistry creates a registry populated with every provider
// in the built-in model catalog, wired to its API key environment variables.
func NewBuiltInProviderRegistry() *ProviderRegistry {
	registry := NewProviderRegistry()
	for _, providerID := range GetProviders() {
		if err := registry.Register(NewSpecProvider(ProviderSpec{
			ID:      providerID,
			Name:    providerID,
			EnvKeys: APIKeyEnvVars(providerID),
			Models:  GetModels(providerID),
		})); err != nil {
			panic(err)
		}
	}
	return registry
}

// Register adds or replaces the provider registered for its ID.
func (registry *ProviderRegistry) Register(provider *Provider) error {
	if registry == nil {
		return errors.New("provider registry is nil")
	}
	if provider == nil {
		return errors.New("provider is nil")
	}
	if provider.ID() == "" {
		return errors.New("provider ID is empty")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.providers[provider.ID()] = provider
	return nil
}

// Get returns the provider registered for the ID.
func (registry *ProviderRegistry) Get(providerID string) (*Provider, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	provider, ok := registry.providers[providerID]
	return provider, ok
}

// Providers returns registered providers ordered by ID.
func (registry *ProviderRegistry) Providers() []*Provider {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	providers := make([]*Provider, 0, len(registry.providers))
	for _, provider := range registry.providers {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ID() < providers[j].ID()
	})
	return providers
}

// SetOverride stores the override applied to every request routed through the
// provider, replacing any previous override. In-flight requests keep the
// override they resolved with; set overrides during startup when possible.
func (registry *ProviderRegistry) SetOverride(providerID string, override ProviderOverride) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.overrides[providerID] = override
}

// ClearOverride removes the provider's override.
func (registry *ProviderRegistry) ClearOverride(providerID string) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()

	delete(registry.overrides, providerID)
}

// ResolveRequest applies the provider's spec and override to one request and
// returns the model and options the adapter should see. A model whose
// provider is not registered passes through unchanged except for the legacy
// environment API key lookup, so hand-built models keep working.
func (registry *ProviderRegistry) ResolveRequest(model Model, options StreamOptions) (Model, StreamOptions) {
	if registry == nil {
		return model, resolveLegacyAPIKey(model, options)
	}

	registry.mu.RLock()
	provider, ok := registry.providers[model.Provider]
	override := registry.overrides[model.Provider]
	registry.mu.RUnlock()

	if !ok {
		return model, resolveLegacyAPIKey(model, options)
	}
	return provider.resolve(model, options, override)
}

// AuthStatus reports the provider's credential state. The second return value
// is false when the provider is not registered. env supplies request-scoped
// environment overrides and may be nil.
func (registry *ProviderRegistry) AuthStatus(providerID string, env ProviderEnv) (AuthStatus, bool) {
	if registry == nil {
		return AuthStatus{}, false
	}
	registry.mu.RLock()
	provider, ok := registry.providers[providerID]
	override := registry.overrides[providerID]
	registry.mu.RUnlock()

	if !ok {
		return AuthStatus{}, false
	}
	return provider.authStatus(env, override), true
}

// resolveLegacyAPIKey fills the API key from the provider's environment
// variables, matching the pre-registry client behavior.
func resolveLegacyAPIKey(model Model, options StreamOptions) StreamOptions {
	if strings.TrimSpace(options.APIKey) == "" {
		options.APIKey = GetEnvAPIKeyWithEnv(model.Provider, options.Env)
	}
	return options
}
