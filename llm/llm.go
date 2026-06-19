// Package llm provides the batteries-included entry point for model inference.
// It keeps a ready-to-use default client for common calls while the lower-level
// implementation remains internal to the module.
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
	UserContent                    = core.UserContent
	AssistantContent               = core.AssistantContent
	ToolResultContent              = core.ToolResultContent
	TextContent                    = core.TextContent
	ThinkingContent                = core.ThinkingContent
	ImageContent                   = core.ImageContent
	ToolCall                       = core.ToolCall
	Message                        = core.Message
	UserMessage                    = core.UserMessage
	AssistantMessage               = core.AssistantMessage
	ToolResultMessage              = core.ToolResultMessage
	ToolDefinition                 = core.ToolDefinition
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
	StreamOptions                  = core.StreamOptions
	Client                         = core.Client
	EventType                      = core.EventType
	Event                          = core.Event
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

// ValidateToolCall validates and coerces a tool call against its definition.
func ValidateToolCall(tools []ToolDefinition, toolCall ToolCall) (map[string]any, error) {
	return core.ValidateToolCall(tools, toolCall)
}

// ValidateToolArguments validates and coerces one tool call against tool.
func ValidateToolArguments(tool ToolDefinition, toolCall ToolCall) (map[string]any, error) {
	return core.ValidateToolArguments(tool, toolCall)
}

// TransformMessages prepares history for replay against model.
func TransformMessages(messages []Message, model Model, normalizeToolCallID func(string) string) []Message {
	return core.TransformMessages(messages, model, normalizeToolCallID)
}

// ParseToolArguments parses and repairs streamed tool argument JSON.
func ParseToolArguments(raw string) (map[string]any, error) {
	return core.ParseToolArguments(raw)
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
