package llm

import (
	"errors"
	"fmt"
)

// ModelCompatibility is implemented by protocol-specific compatibility
// configurations. It keeps Model independent from any one provider protocol
// while allowing registration and adapters to verify type/protocol agreement.
type ModelCompatibility interface {
	Protocol() Protocol
	// clone returns a fully independent deep copy, including every pointer field,
	// so a registered or returned Model shares no mutable state with its source.
	// Keeping this next to each concrete type localizes the per-field cloning to
	// the struct it belongs to and keeps cloneModel free of a protocol type switch.
	clone() ModelCompatibility
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

func (compatibility *OpenAICompletionsCompatibility) clone() ModelCompatibility {
	if compatibility == nil {
		return nil
	}
	clone := *compatibility
	clone.SupportsStore = clonePointer(compatibility.SupportsStore)
	clone.SupportsDeveloperRole = clonePointer(compatibility.SupportsDeveloperRole)
	clone.SupportsReasoningEffort = clonePointer(compatibility.SupportsReasoningEffort)
	clone.SupportsStrictMode = clonePointer(compatibility.SupportsStrictMode)
	clone.RequiresReasoningContentOnAssistantMessages = clonePointer(compatibility.RequiresReasoningContentOnAssistantMessages)
	clone.RequiresThinkingAsText = clonePointer(compatibility.RequiresThinkingAsText)
	clone.ZAIToolStream = clonePointer(compatibility.ZAIToolStream)
	return &clone
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

func (compatibility *AnthropicMessagesCompatibility) clone() ModelCompatibility {
	if compatibility == nil {
		return nil
	}
	clone := *compatibility
	clone.SupportsTemperature = clonePointer(compatibility.SupportsTemperature)
	clone.SupportsCacheControl = clonePointer(compatibility.SupportsCacheControl)
	clone.SupportsCacheControlTools = clonePointer(compatibility.SupportsCacheControlTools)
	clone.ForceAdaptiveThinking = clonePointer(compatibility.ForceAdaptiveThinking)
	clone.AllowEmptySignature = clonePointer(compatibility.AllowEmptySignature)
	return &clone
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
