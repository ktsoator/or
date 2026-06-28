package llm

import (
	"encoding/json"
	"fmt"
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

// OpenAICompletionsCompatibility describes differences between providers that
// implement an OpenAI-compatible Chat Completions endpoint. Pointer booleans
// distinguish an explicit false value from an unspecified provider default.
type OpenAICompletionsCompatibility struct {
	SupportsStore                               *bool  `json:"supportsStore,omitempty"`
	SupportsDeveloperRole                       *bool  `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort                     *bool  `json:"supportsReasoningEffort,omitempty"`
	MaxTokensField                              string `json:"maxTokensField,omitempty"`
	SupportsStrictMode                          *bool  `json:"supportsStrictMode,omitempty"`
	RequiresReasoningContentOnAssistantMessages *bool  `json:"requiresReasoningContentOnAssistantMessages,omitempty"`
	// RequiresThinkingAsText makes replayed assistant turns carry thinking as a
	// leading text content block instead of a provider reasoning field, for
	// endpoints that reject reasoning fields on input.
	RequiresThinkingAsText *bool  `json:"requiresThinkingAsText,omitempty"`
	ThinkingFormat         string `json:"thinkingFormat,omitempty"`
	ZAIToolStream          *bool  `json:"zaiToolStream,omitempty"`
}

// Protocol identifies the API protocol whose request and message dialect this
// compatibility configuration describes.
func (*OpenAICompletionsCompatibility) Protocol() Protocol {
	return ProtocolOpenAICompletions
}

// AnthropicMessagesCompatibility describes differences between providers that
// implement an Anthropic Messages-compatible endpoint. Pointer booleans
// distinguish an explicit false value from an unspecified provider default.
// Anthropic-compatible vendors (e.g. MiniMax) are served by pointing the base
// URL at their endpoint; most need no overrides at all.
type AnthropicMessagesCompatibility struct {
	SupportsTemperature       *bool `json:"supportsTemperature,omitempty"`
	SupportsCacheControl      *bool `json:"supportsCacheControl,omitempty"`
	SupportsCacheControlTools *bool `json:"supportsCacheControlOnTools,omitempty"`
	ForceAdaptiveThinking     *bool `json:"forceAdaptiveThinking,omitempty"`
	AllowEmptySignature       *bool `json:"allowEmptySignature,omitempty"`
}

// Protocol identifies the API protocol whose request and message dialect this
// compatibility configuration describes.
func (*AnthropicMessagesCompatibility) Protocol() Protocol {
	return ProtocolAnthropicMessages
}

// ModelCompatibility is implemented by protocol-specific compatibility
// configurations. It keeps Model independent from any one provider protocol
// while allowing registration and adapters to verify type/protocol agreement.
type ModelCompatibility interface {
	Protocol() Protocol
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
// The protocol acts as the discriminator, mirroring pi's Model<TApi> conditional
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
