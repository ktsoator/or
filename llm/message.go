package llm

import (
	"encoding/json"
	"strings"
	"time"
)

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

/*
Content roles

The three interfaces below are compile-time placement rules. A content block
implements only the roles it is allowed to appear in.

                 UserMessage   AssistantMessage   ToolResultMessage
TextContent          yes             yes                 yes
ImageContent         yes             no                  yes
ThinkingContent      no              yes                 no
ToolCall             no              yes                 no
*/

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

/*
Content blocks are the provider-neutral payloads inside message bodies.
Adapters translate them to provider-specific block formats when sending
history and rebuild the same block types while reading response streams.
*/

// TextContent represents plain text.
type TextContent struct {
	Text          string `json:"text"`
	TextSignature string `json:"textSignature,omitempty"`
}

func (*TextContent) isUserContent()       {}
func (*TextContent) isAssistantContent()  {}
func (*TextContent) isToolResultContent() {}

// ImageContent represents a base64-encoded image.
type ImageContent struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

func (*ImageContent) isUserContent()       {}
func (*ImageContent) isToolResultContent() {}

// ThinkingContent represents model reasoning content.
type ThinkingContent struct {
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

func (*ThinkingContent) isAssistantContent() {}

// ToolCall describes a request to invoke a named tool with JSON arguments.
type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

func (*ToolCall) isAssistantContent() {}

/*
Message turns are the conversation items passed between callers and adapters.
UserMessage is caller input, AssistantMessage is model output, and
ToolResultMessage answers an assistant ToolCall by ID.
*/

// Message is one item in the conversation context.
type Message interface {
	isMessage()
}

// UserMessage contains content supplied by the user.
type UserMessage struct {
	Content []UserContent `json:"content"`
}

func (*UserMessage) isMessage() {}

// AssistantMessage is the final or partial response returned by a provider.
type AssistantMessage struct {
	// Content contains the model output blocks: text, thinking, and tool calls.
	Content []AssistantContent `json:"content"`

	// --- Response metadata ---
	// Protocol is the wire protocol used for this response.
	Protocol Protocol `json:"protocol"`
	// Provider is the model provider that produced this response.
	Provider string `json:"provider"`
	// Model is the requested model ID.
	Model string `json:"model"`
	// ResponseModel is the model name reported by the provider, when it differs
	// from or further qualifies the requested model.
	ResponseModel string `json:"responseModel,omitempty"`
	// ResponseID is the provider's unique identifier for this response.
	ResponseID string `json:"responseId,omitempty"`
	// Usage records token consumption and calculated cost for this response.
	Usage Usage `json:"usage"`
	// StopReason explains why generation stopped.
	StopReason StopReason `json:"stopReason"`
	// ErrorMessage stores the provider or runtime error for failed responses.
	ErrorMessage string `json:"errorMessage,omitempty"`
	// Diagnostics records non-fatal events (failures recovered from, degraded
	// results) that occurred while producing this response. It is nil for a
	// clean response.
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	// Timestamp is the Unix millisecond time when the response object was
	// created.
	Timestamp int64 `json:"timestamp"`
}

func (*AssistantMessage) isMessage() {}

// Text concatenates the text from every text block in the message, in order. It
// ignores thinking and tool-call blocks, returning "" when there is no text.
func (message *AssistantMessage) Text() string {
	if message == nil {
		return ""
	}
	var builder strings.Builder
	for _, content := range message.Content {
		if text, ok := content.(*TextContent); ok && text != nil {
			builder.WriteString(text.Text)
		}
	}
	return builder.String()
}

// ToolCalls returns every tool call in the message, in order. It returns nil
// when the message requested no tools.
func (message *AssistantMessage) ToolCalls() []ToolCall {
	if message == nil {
		return nil
	}
	var calls []ToolCall
	for _, content := range message.Content {
		if call, ok := content.(*ToolCall); ok && call != nil {
			calls = append(calls, *call)
		}
	}
	return calls
}

// NewAssistantMessage initializes provider-independent response metadata.
func NewAssistantMessage(model Model) AssistantMessage {
	return AssistantMessage{
		Protocol:  model.Protocol,
		Provider:  model.Provider,
		Model:     model.ID,
		Timestamp: time.Now().UnixMilli(),
	}
}

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

// Context
//
//	├── SystemPrompt
//	├── Messages
//	│   ├── UserMessage
//	│   │   └── []UserContent
//	│   │       ├── TextContent
//	│   │       └── ImageContent
//	│   │
//	│   ├── AssistantMessage
//	│   │   └── []AssistantContent
//	│   │       ├── TextContent
//	│   │       ├── ThinkingContent
//	│   │       └── ToolCall
//	│   │
//	│   └── ToolResultMessage
//	│       └── []ToolResultContent
//	│           ├── TextContent
//	│           └── ImageContent
//	│
//	└── Tools []ToolDefinition

// Context contains the prompt, conversation history, and available tools.
type Context struct {
	SystemPrompt string           `json:"systemPrompt,omitempty"`
	Messages     []Message        `json:"messages"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}
