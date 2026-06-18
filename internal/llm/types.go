package llm

import (
	"encoding/json"
	"fmt"
	"time"
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
	ModelInputText  ModelInput = "text"
	ModelInputImage ModelInput = "image"
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

// UserContent is content that can appear in a user message.
type UserContent interface {
	isUserContent()
}

// AssistantContent is content that can appear in an assistant message.
type AssistantContent interface {
	isAssistantContent()
}

// ToolResultContent is content that can appear in a tool result message.
type ToolResultContent interface {
	isToolResultContent()
}

// TextContent represents plain text.
type TextContent struct {
	Text          string `json:"text"`
	TextSignature string `json:"textSignature,omitempty"`
}

func (*TextContent) isUserContent()       {}
func (*TextContent) isAssistantContent()  {}
func (*TextContent) isToolResultContent() {}

// ThinkingContent represents model reasoning content.
type ThinkingContent struct {
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

func (*ThinkingContent) isAssistantContent() {}

// ImageContent represents a base64-encoded image.
type ImageContent struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

func (*ImageContent) isUserContent()       {}
func (*ImageContent) isToolResultContent() {}

// ToolCall describes a request to invoke a named tool with JSON arguments.
type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

func (*ToolCall) isAssistantContent() {}

// Message is one item in the conversation context.
type Message interface {
	isMessage()
}

// UserMessage contains content supplied by the user.
type UserMessage struct {
	Content []UserContent `json:"content"`
}

func (*UserMessage) isMessage() {}

// ToolResultMessage contains the result of an assistant tool call.
type ToolResultMessage struct {
	ToolCallID string              `json:"toolCallId"`
	ToolName   string              `json:"toolName"`
	Content    []ToolResultContent `json:"content"`
	IsError    bool                `json:"isError"`
}

func (*ToolResultMessage) isMessage() {}

// ToolDefinition describes a tool that the model may call.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Context contains the prompt, conversation history, and available tools.
type Context struct {
	SystemPrompt string           `json:"systemPrompt,omitempty"`
	Messages     []Message        `json:"messages"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}

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
	ThinkingFormat                              string `json:"thinkingFormat,omitempty"`
	ZAIToolStream                               *bool  `json:"zaiToolStream,omitempty"`
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
	ID               string                         `json:"id"`
	Name             string                         `json:"name"`
	Protocol         Protocol                       `json:"protocol"`
	Provider         string                         `json:"provider"`
	BaseURL          string                         `json:"baseUrl"`
	Reasoning        bool                           `json:"reasoning"`
	ThinkingLevelMap map[ModelThinkingLevel]*string `json:"thinkingLevelMap,omitempty"`
	Input            []ModelInput                   `json:"input"`
	Cost             ModelCost                      `json:"cost"`
	ContextWindow    int64                          `json:"contextWindow"`
	MaxTokens        int64                          `json:"maxTokens"`
	Headers          map[string]string              `json:"headers,omitempty"`
	Compatibility    ModelCompatibility             `json:"compat,omitempty"`
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

// Usage records token consumption for one assistant response.
type Usage struct {
	Input       int64     `json:"input"`
	Output      int64     `json:"output"`
	CacheRead   int64     `json:"cacheRead"`
	CacheWrite  int64     `json:"cacheWrite"`
	TotalTokens int64     `json:"totalTokens"`
	Cost        UsageCost `json:"cost"`
}

// UsageCost breaks down the US dollar cost of one response by token category.
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// StopReason explains why the model stopped generating a response.
type StopReason string

const (
	// StopReasonStop marks a normal completion.
	StopReasonStop StopReason = "stop"
	// StopReasonLength marks truncation by the max output token limit.
	StopReasonLength StopReason = "length"
	// StopReasonToolUse marks a stop to let the caller execute tool calls.
	StopReasonToolUse StopReason = "toolUse"
	// StopReasonError marks a provider or runtime failure.
	StopReasonError StopReason = "error"
	// StopReasonAborted marks a cancelled request.
	StopReasonAborted StopReason = "aborted"
)

// AssistantMessage is the final or partial response returned by a provider.
type AssistantMessage struct {
	Content       []AssistantContent `json:"content"`
	Protocol      Protocol           `json:"protocol"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	ResponseModel string             `json:"responseModel,omitempty"`
	ResponseID    string             `json:"responseId,omitempty"`
	Usage         Usage              `json:"usage"`
	StopReason    StopReason         `json:"stopReason"`
	ErrorMessage  string             `json:"errorMessage,omitempty"`
	Timestamp     int64              `json:"timestamp"`
}

func (*AssistantMessage) isMessage() {}

// NewAssistantMessage initializes provider-independent response metadata.
func NewAssistantMessage(model Model) AssistantMessage {
	return AssistantMessage{
		Protocol:  model.Protocol,
		Provider:  model.Provider,
		Model:     model.ID,
		Timestamp: time.Now().UnixMilli(),
	}
}
