package llm

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Protocol identifies the API protocol used to communicate with a model.
type Protocol string

const (
	ProtocolOpenAICompletions Protocol = "openai-completions"
	ProtocolAnthropicMessages Protocol = "anthropic-messages"
)

// ModelInput identifies an input modality accepted by a model.
type ModelInput string

const (
	Text  ModelInput = "text"
	Image ModelInput = "image"
)

// ModelThinkingLevel is a provider-independent reasoning effort level.
type ModelThinkingLevel string

const (
	ModelThinkingOff     ModelThinkingLevel = "off"
	ModelThinkingMinimal ModelThinkingLevel = "minimal"
	ModelThinkingLow     ModelThinkingLevel = "low"
	ModelThinkingMedium  ModelThinkingLevel = "medium"
	ModelThinkingHigh    ModelThinkingLevel = "high"
	ModelThinkingXHigh   ModelThinkingLevel = "xhigh"
)

// ThinkingDisplay controls how a reasoning model returns its thinking. It does
// not change whether the model reasons or what it is billed; it only governs
// what travels back. Only Anthropic-protocol models honor it today.
type ThinkingDisplay string

const (
	// ThinkingDisplaySummarized returns summarized thinking text in the response.
	ThinkingDisplaySummarized ThinkingDisplay = "summarized"
	// ThinkingDisplayOmitted redacts the thinking text but still returns the
	// signature needed for multi-turn tool-use continuity. Use it for backends
	// that never surface reasoning, trading the thinking text for lower
	// time-to-first-token and a lighter response.
	ThinkingDisplayOmitted ThinkingDisplay = "omitted"
)

// ModelCost stores prices in US dollars per million tokens.
type ModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// Model identifies a model, its provider endpoint, capabilities, limits, and
// pricing. ThinkingLevelMap values are provider-specific; nil marks a level as
// unsupported while a missing key uses the provider default.
type Model struct {
	// Identity: how the model is named and grouped.
	ID       string `json:"id"`       // stable identifier sent to the provider
	Name     string `json:"name"`     // human-readable display name
	Provider string `json:"provider"` // vendor key, e.g. "anthropic", "openai"

	// Routing: how to talk to the model. Protocol is the discriminator the
	// Client uses to pick an adapter (see Client.Stream); BaseURL and Headers
	// let a compatible vendor reuse a protocol against its own endpoint.
	Protocol Protocol          `json:"protocol"`
	BaseURL  string            `json:"baseUrl"`
	Headers  map[string]string `json:"headers,omitempty"`

	// Capabilities: what the model can do and its size limits.
	Reasoning bool `json:"reasoning"` // whether the model can produce thinking
	// ThinkingLevelMap maps a provider-independent level to the provider's own
	// value; a nil value marks a level as unsupported, a missing key falls back
	// to the provider default.
	ThinkingLevelMap map[ModelThinkingLevel]*string `json:"thinkingLevelMap,omitempty"`
	Input            []ModelInput                   `json:"input"`         // accepted modalities: text, image
	ContextWindow    int64                          `json:"contextWindow"` // max total tokens (input + output)
	MaxTokens        int64                          `json:"maxTokens"`     // max tokens the model may generate

	// Pricing and per-provider quirks.
	Cost ModelCost `json:"cost"`
	// Compatibility carries protocol-specific overrides for vendors that
	// deviate from the reference API. Its concrete type is selected at decode
	// time by Protocol (see UnmarshalJSON below).
	Compatibility ModelCompatibility `json:"compat,omitempty"`
}

// UnmarshalJSON restores the concrete compatibility type selected by Protocol.
// The protocol acts as the discriminator, selecting the concrete per-protocol
// compatibility type at runtime.
func (model *Model) UnmarshalJSON(data []byte) error {
	if model == nil {
		return fmt.Errorf("cannot unmarshal model into nil receiver")
	}

	type modelAlias Model
	wire := struct {
		*modelAlias
		Compatibility json.RawMessage `json:"compat"`
	}{modelAlias: (*modelAlias)(model)}

	*model = Model{}

	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode model: %w", err)
	}
	if len(wire.Compatibility) == 0 || isJSONNull(wire.Compatibility) {
		model.Compatibility = nil
		return nil
	}

	switch model.Protocol {
	case ProtocolOpenAICompletions:
		var compatibility OpenAICompletionsCompatibility
		if err := json.Unmarshal(wire.Compatibility, &compatibility); err != nil {
			return fmt.Errorf("decode %s compatibility: %w", model.Protocol, err)
		}
		model.Compatibility = &compatibility
		return nil
	case ProtocolAnthropicMessages:
		var compatibility AnthropicMessagesCompatibility
		if err := json.Unmarshal(wire.Compatibility, &compatibility); err != nil {
			return fmt.Errorf("decode %s compatibility: %w", model.Protocol, err)
		}
		model.Compatibility = &compatibility
		return nil
	default:
		return fmt.Errorf("decode model: unsupported compatibility protocol %q", model.Protocol)
	}
}

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
		maps.Copy(clone.Headers, model.Headers)
	}
	if model.ThinkingLevelMap != nil {
		clone.ThinkingLevelMap = make(map[ModelThinkingLevel]*string, len(model.ThinkingLevelMap))
		for level, value := range model.ThinkingLevelMap {
			clone.ThinkingLevelMap[level] = clonePointer(value)
		}
	}
	if model.Compatibility != nil {
		clone.Compatibility = model.Compatibility.clone()
	}
	return clone
}

// clonePointer copies a pointer to a value-semantic type. It is intended for
// scalar configuration fields such as *string, *bool, and numeric pointers. Do
// not use it as a deep copy for slices, maps, or structs that contain reference
// fields; clone those explicitly instead.
func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
