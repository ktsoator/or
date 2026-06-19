package llm

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
)

// extendedThinkingLevels lists every thinking level from off to highest, in order.
var extendedThinkingLevels = []ModelThinkingLevel{
	ModelThinkingOff,
	ModelThinkingMinimal,
	ModelThinkingLow,
	ModelThinkingMedium,
	ModelThinkingHigh,
	ModelThinkingXHigh,
}

// SupportedThinkingLevels returns the thinking levels a model accepts. A
// non-reasoning model supports only "off". For reasoning models, a level mapped
// to nil is unsupported, and "xhigh" is supported only when explicitly mapped.
func SupportedThinkingLevels(model Model) []ModelThinkingLevel {
	if !model.Reasoning {
		return []ModelThinkingLevel{ModelThinkingOff}
	}

	var levels []ModelThinkingLevel
	for _, level := range extendedThinkingLevels {
		mapped, present := model.ThinkingLevelMap[level]
		if present && mapped == nil {
			continue
		}
		if level == ModelThinkingXHigh && !present {
			continue
		}
		levels = append(levels, level)
	}
	return levels
}

// ClampThinkingLevel adjusts a requested level to the nearest one the model
// supports: it prefers the requested level, then steps up, then down, and falls
// back to the lowest supported level (or "off").
func ClampThinkingLevel(model Model, level ModelThinkingLevel) ModelThinkingLevel {
	available := SupportedThinkingLevels(model)
	if slices.Contains(available, level) {
		return level
	}

	requested := slices.Index(extendedThinkingLevels, level)
	if requested == -1 {
		if len(available) > 0 {
			return available[0]
		}
		return ModelThinkingOff
	}
	for i := requested; i < len(extendedThinkingLevels); i++ {
		if slices.Contains(available, extendedThinkingLevels[i]) {
			return extendedThinkingLevels[i]
		}
	}
	for i := requested - 1; i >= 0; i-- {
		if slices.Contains(available, extendedThinkingLevels[i]) {
			return extendedThinkingLevels[i]
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return ModelThinkingOff
}

// ModelRegistry stores models by provider and model ID. It is safe for
// concurrent access and returns defensive copies of registered models.
type ModelRegistry struct {
	mu     sync.RWMutex
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

// CalculateCost returns the US dollar cost of usage at the model's prices. Model
// costs are quoted per million tokens.
func CalculateCost(model Model, usage Usage) UsageCost {
	const perMillion = 1_000_000.0
	cost := UsageCost{
		Input:      model.Cost.Input / perMillion * float64(usage.Input),
		Output:     model.Cost.Output / perMillion * float64(usage.Output),
		CacheRead:  model.Cost.CacheRead / perMillion * float64(usage.CacheRead),
		CacheWrite: model.Cost.CacheWrite / perMillion * float64(usage.CacheWrite),
	}
	cost.Total = cost.Input + cost.Output + cost.CacheRead + cost.CacheWrite
	return cost
}

func cloneModel(model Model) Model {
	clone := model
	clone.Input = append([]ModelInput(nil), model.Input...)
	if model.Headers != nil {
		clone.Headers = make(map[string]string, len(model.Headers))
		for name, value := range model.Headers {
			clone.Headers[name] = value
		}
	}
	if model.ThinkingLevelMap != nil {
		clone.ThinkingLevelMap = make(map[ModelThinkingLevel]*string, len(model.ThinkingLevelMap))
		for level, value := range model.ThinkingLevelMap {
			clone.ThinkingLevelMap[level] = cloneStringPointer(value)
		}
	}
	switch compatibility := model.Compatibility.(type) {
	case *OpenAICompletionsCompatibility:
		if compatibility != nil {
			compatibilityClone := *compatibility
			compatibilityClone.SupportsStore = cloneBoolPointer(compatibility.SupportsStore)
			compatibilityClone.SupportsDeveloperRole = cloneBoolPointer(compatibility.SupportsDeveloperRole)
			compatibilityClone.SupportsReasoningEffort = cloneBoolPointer(compatibility.SupportsReasoningEffort)
			compatibilityClone.SupportsStrictMode = cloneBoolPointer(compatibility.SupportsStrictMode)
			compatibilityClone.RequiresThinkingAsText = cloneBoolPointer(compatibility.RequiresThinkingAsText)
			compatibilityClone.RequiresReasoningContentOnAssistantMessages = cloneBoolPointer(
				compatibility.RequiresReasoningContentOnAssistantMessages,
			)
			compatibilityClone.ZAIToolStream = cloneBoolPointer(compatibility.ZAIToolStream)
			clone.Compatibility = &compatibilityClone
		}
	case *AnthropicMessagesCompatibility:
		if compatibility != nil {
			compatibilityClone := *compatibility
			compatibilityClone.SupportsTemperature = cloneBoolPointer(compatibility.SupportsTemperature)
			compatibilityClone.SupportsCacheControl = cloneBoolPointer(compatibility.SupportsCacheControl)
			compatibilityClone.SupportsCacheControlTools = cloneBoolPointer(compatibility.SupportsCacheControlTools)
			compatibilityClone.ForceAdaptiveThinking = cloneBoolPointer(compatibility.ForceAdaptiveThinking)
			compatibilityClone.AllowEmptySignature = cloneBoolPointer(compatibility.AllowEmptySignature)
			clone.Compatibility = &compatibilityClone
		}
	}
	return clone
}

func validateModelCompatibility(model Model) error {
	if model.Compatibility == nil {
		return nil
	}

	switch compatibility := model.Compatibility.(type) {
	case *OpenAICompletionsCompatibility:
		if compatibility == nil {
			return errors.New("model compatibility is a typed nil")
		}
	case *AnthropicMessagesCompatibility:
		if compatibility == nil {
			return errors.New("model compatibility is a typed nil")
		}
	default:
		return fmt.Errorf("unsupported model compatibility type %T", model.Compatibility)
	}

	if model.Compatibility.Protocol() != model.Protocol {
		return fmt.Errorf(
			"model compatibility protocol %q does not match model protocol %q",
			model.Compatibility.Protocol(),
			model.Protocol,
		)
	}
	return nil
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
