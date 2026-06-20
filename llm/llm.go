package llm

import (
	"context"

	core "github.com/ktsoator/or/internal/llm"
	"github.com/ktsoator/or/internal/llm/providers/anthropic"
	"github.com/ktsoator/or/internal/llm/providers/openai"
)

// Core conversation, model, and streaming types are aliases so callers only
// need this package.
type (
	Protocol                       = core.Protocol
	ModelInput                     = core.ModelInput
	ModelThinkingLevel             = core.ModelThinkingLevel
	ThinkingDisplay                = core.ThinkingDisplay
	UserContent                    = core.UserContent
	AssistantContent               = core.AssistantContent
	TextContent                    = core.TextContent
	ThinkingContent                = core.ThinkingContent
	ImageContent                   = core.ImageContent
	Message                        = core.Message
	UserMessage                    = core.UserMessage
	AssistantMessage               = core.AssistantMessage
	Context                        = core.Context
	ModelCost                      = core.ModelCost
	ModelCompatibility             = core.ModelCompatibility
	OpenAICompletionsCompatibility = core.OpenAICompletionsCompatibility
	AnthropicMessagesCompatibility = core.AnthropicMessagesCompatibility
	Model                          = core.Model
	Usage                          = core.Usage
	UsageCost                      = core.UsageCost
	StopReason                     = core.StopReason
	ProviderEnv                    = core.ProviderEnv
	ProtocolStreamOptions          = core.ProtocolStreamOptions
	AnthropicToolChoice            = core.AnthropicToolChoice
	AnthropicToolChoiceMode        = core.AnthropicToolChoiceMode
	AnthropicToolChoiceTool        = core.AnthropicToolChoiceTool
	AnthropicStreamOptions         = core.AnthropicStreamOptions
	OpenAIToolChoice               = core.OpenAIToolChoice
	OpenAIToolChoiceMode           = core.OpenAIToolChoiceMode
	OpenAIToolChoiceFunction       = core.OpenAIToolChoiceFunction
	OpenAICompletionsStreamOptions = core.OpenAICompletionsStreamOptions
	StreamOptions                  = core.StreamOptions
	Client                         = core.Client
	EventType                      = core.EventType
	Event                          = core.Event
	Diagnostic                     = core.Diagnostic
)

const (
	ProtocolOpenAICompletions = core.ProtocolOpenAICompletions
	ProtocolAnthropicMessages = core.ProtocolAnthropicMessages

	Text  = core.Text
	Image = core.Image

	ModelThinkingOff     = core.ModelThinkingOff
	ModelThinkingMinimal = core.ModelThinkingMinimal
	ModelThinkingLow     = core.ModelThinkingLow
	ModelThinkingMedium  = core.ModelThinkingMedium
	ModelThinkingHigh    = core.ModelThinkingHigh
	ModelThinkingXHigh   = core.ModelThinkingXHigh

	ThinkingDisplaySummarized = core.ThinkingDisplaySummarized
	ThinkingDisplayOmitted    = core.ThinkingDisplayOmitted

	AnthropicToolChoiceAuto = core.AnthropicToolChoiceAuto
	AnthropicToolChoiceAny  = core.AnthropicToolChoiceAny
	AnthropicToolChoiceNone = core.AnthropicToolChoiceNone

	OpenAIToolChoiceAuto     = core.OpenAIToolChoiceAuto
	OpenAIToolChoiceNone     = core.OpenAIToolChoiceNone
	OpenAIToolChoiceRequired = core.OpenAIToolChoiceRequired

	StopReasonStop    = core.StopReasonStop
	StopReasonLength  = core.StopReasonLength
	StopReasonToolUse = core.StopReasonToolUse
	StopReasonError   = core.StopReasonError
	StopReasonAborted = core.StopReasonAborted

	EventStart         = core.EventStart
	EventTextStart     = core.EventTextStart
	EventTextDelta     = core.EventTextDelta
	EventTextEnd       = core.EventTextEnd
	EventThinkingStart = core.EventThinkingStart
	EventThinkingDelta = core.EventThinkingDelta
	EventThinkingEnd   = core.EventThinkingEnd
	EventToolCallStart = core.EventToolCallStart
	EventToolCallDelta = core.EventToolCallDelta
	EventToolCallEnd   = core.EventToolCallEnd
	EventDone          = core.EventDone
	EventError         = core.EventError
)

var defaultClient = NewClient()

// NewClient returns an isolated client with all built-in protocol adapters
// registered. Most callers can use the package-level Stream and Complete
// functions instead.
func NewClient() *Client {
	registry := core.NewRegistry()
	mustRegister(registry, openai.NewAdapter(nil))
	mustRegister(registry, anthropic.NewAdapter(nil))
	return core.NewClient(registry)
}

// Stream uses the default client to start a streaming model request.
func Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error) {
	return defaultClient.Stream(ctx, model, input, options)
}

// Complete uses the default client and returns the final assistant message.
func Complete(ctx context.Context, model Model, input Context, options StreamOptions) (AssistantMessage, error) {
	return defaultClient.Complete(ctx, model, input, options)
}

// LookupModel returns a model from the built-in catalog.
func LookupModel(provider, modelID string) (Model, bool) {
	return core.LookupModel(provider, modelID)
}

// GetModel returns a model from the built-in catalog and panics when unknown.
func GetModel(provider, modelID string) Model {
	return core.GetModel(provider, modelID)
}

// GetProviders returns the provider IDs in the built-in model catalog.
func GetProviders() []string {
	return core.GetProviders()
}

// GetModels returns the built-in models for provider.
func GetModels(provider string) []Model {
	return core.GetModels(provider)
}

// SupportedThinkingLevels returns the reasoning levels accepted by model.
func SupportedThinkingLevels(model Model) []ModelThinkingLevel {
	return core.SupportedThinkingLevels(model)
}

// ClampThinkingLevel returns the nearest reasoning level accepted by model.
func ClampThinkingLevel(model Model, level ModelThinkingLevel) ModelThinkingLevel {
	return core.ClampThinkingLevel(model, level)
}

// CalculateCost calculates the model cost for usage.
func CalculateCost(model Model, usage Usage) UsageCost {
	return core.CalculateCost(model, usage)
}

// TransformMessages prepares history for replay against model.
func TransformMessages(messages []Message, model Model, normalizeToolCallID func(string) string) []Message {
	return core.TransformMessages(messages, model, normalizeToolCallID)
}

// IsContextOverflow reports whether a response indicates a context overflow.
func IsContextOverflow(message AssistantMessage, contextWindow int64) bool {
	return core.IsContextOverflow(message, contextWindow)
}

func mustRegister(registry *core.Registry, adapter core.ProtocolAdapter) {
	if err := registry.Register(adapter); err != nil {
		panic(err)
	}
}
