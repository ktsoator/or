package llm

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ModelRegistry stores models by provider and model ID. It is safe for
// concurrent access and returns defensive copies of registered models.
type ModelRegistry struct {
	mu sync.RWMutex
	// models is keyed by provider, then by model ID. Providers are grouped
	// independently of protocol, so a provider's own model IDs nest under it:
	//
	//   models ┌─ "anthropic" ─┬─ "claude-opus-4-8"     → Model{protocol: anthropic-messages}
	//          │               └─ "claude-sonnet-4-6"   → Model{protocol: anthropic-messages}
	//          │
	//          └─ "deepseek" ──┬─ "deepseek-v4-flash"   → Model{protocol: openai-completions}
	//                          └─ "deepseek-v4-pro"     → Model{protocol: openai-completions}
	models map[string]map[string]Model
}

// NewModelRegistry creates an empty model registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{models: make(map[string]map[string]Model)}
}

// Register adds or replaces a model with the same provider and ID.
func (registry *ModelRegistry) Register(model Model) error {
	if registry == nil {
		return errors.New("model registry is nil")
	}
	if model.Provider == "" {
		return errors.New("model provider is empty")
	}
	if model.ID == "" {
		return errors.New("model ID is empty")
	}
	if model.Protocol == "" {
		return errors.New("model protocol is empty")
	}
	if err := validateModelCompatibility(model); err != nil {
		return err
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	providerModels := registry.models[model.Provider]
	if providerModels == nil {
		providerModels = make(map[string]Model)
		registry.models[model.Provider] = providerModels
	}
	providerModels[model.ID] = cloneModel(model)
	return nil
}

// Get returns a model registered for provider and modelID.
func (registry *ModelRegistry) Get(provider, modelID string) (Model, bool) {
	if registry == nil {
		return Model{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	model, ok := registry.models[provider][modelID]
	if !ok {
		return Model{}, false
	}
	return cloneModel(model), true
}

// Providers returns registered provider IDs in lexical order.
func (registry *ModelRegistry) Providers() []string {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	providers := make([]string, 0, len(registry.models))
	for provider := range registry.models {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

// Models returns a provider's models ordered by model ID.
func (registry *ModelRegistry) Models(provider string) []Model {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	providerModels := registry.models[provider]
	modelIDs := make([]string, 0, len(providerModels))
	for modelID := range providerModels {
		modelIDs = append(modelIDs, modelID)
	}
	sort.Strings(modelIDs)

	models := make([]Model, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, cloneModel(providerModels[modelID]))
	}
	return models
}

// LookupModel returns a model from the package's built-in model registry.
func LookupModel(provider, modelID string) (Model, bool) {
	return builtInModelRegistry.Get(provider, modelID)
}

// GetModel returns a model from the package's built-in model registry. It
// panics when the provider/model pair is unknown. Use LookupModel when the
// identifiers come from dynamic or untrusted input.
func GetModel(provider, modelID string) Model {
	model, ok := LookupModel(provider, modelID)
	if !ok {
		panic(fmt.Sprintf("llm: unknown model %q for provider %q", modelID, provider))
	}
	return model
}

// GetProviders returns all providers in the built-in model registry.
func GetProviders() []string {
	return builtInModelRegistry.Providers()
}

// GetModels returns all built-in models for a provider.
func GetModels(provider string) []Model {
	return builtInModelRegistry.Models(provider)
}
