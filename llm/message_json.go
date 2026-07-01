package llm

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	contentTypeText      = "text"
	contentTypeThinking  = "thinking"
	contentTypeImage     = "image"
	contentTypeToolCall  = "toolCall"
	messageRoleUser      = "user"
	messageRoleAssistant = "assistant"
	messageRoleTool      = "toolResult"
)

type contentHeader struct {
	Type string `json:"type"`
}

type messageHeader struct {
	Role string `json:"role"`
}

type textContentWire struct {
	Type          string `json:"type"`
	Text          string `json:"text"`
	TextSignature string `json:"textSignature,omitempty"`
}

type thinkingContentWire struct {
	Type              string `json:"type"`
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

type imageContentWire struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type toolCallWire struct {
	Type             string         `json:"type"`
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

type userMessageWire struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type assistantMessageWire struct {
	Role          string            `json:"role"`
	Content       []json.RawMessage `json:"content"`
	Protocol      Protocol          `json:"protocol"`
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	ResponseModel string            `json:"responseModel,omitempty"`
	ResponseID    string            `json:"responseId,omitempty"`
	Usage         Usage             `json:"usage"`
	StopReason    StopReason        `json:"stopReason"`
	ErrorMessage  string            `json:"errorMessage,omitempty"`
	Diagnostics   []Diagnostic      `json:"diagnostics,omitempty"`
	Timestamp     int64             `json:"timestamp"`
}

type toolResultMessageWire struct {
	Role       string            `json:"role"`
	ToolCallID string            `json:"toolCallId"`
	ToolName   string            `json:"toolName"`
	Content    []json.RawMessage `json:"content"`
	IsError    bool              `json:"isError"`
}

type contextWire struct {
	SystemPrompt string            `json:"systemPrompt,omitempty"`
	Messages     []json.RawMessage `json:"messages"`
	Tools        []ToolDefinition  `json:"tools,omitempty"`
}

func (content TextContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(textContentWire{
		Type:          contentTypeText,
		Text:          content.Text,
		TextSignature: content.TextSignature,
	})
}

func (content ThinkingContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(thinkingContentWire{
		Type:              contentTypeThinking,
		Thinking:          content.Thinking,
		ThinkingSignature: content.ThinkingSignature,
		Redacted:          content.Redacted,
	})
}

func (content ImageContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(imageContentWire{
		Type:     contentTypeImage,
		Data:     content.Data,
		MIMEType: content.MIMEType,
	})
}

func (content ToolCall) MarshalJSON() ([]byte, error) {
	arguments := content.Arguments
	if arguments == nil {
		arguments = make(map[string]any)
	}
	return json.Marshal(toolCallWire{
		Type:             contentTypeToolCall,
		ID:               content.ID,
		Name:             content.Name,
		Arguments:        arguments,
		ThoughtSignature: content.ThoughtSignature,
	})
}

func (message UserMessage) MarshalJSON() ([]byte, error) {
	content, err := marshalUserContent(message.Content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(userMessageWire{Role: messageRoleUser, Content: content})
}

func (message *UserMessage) UnmarshalJSON(data []byte) error {
	if message == nil {
		return errors.New("cannot unmarshal user message into nil receiver")
	}
	var wire userMessageWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode user message: %w", err)
	}
	if wire.Role != messageRoleUser {
		return fmt.Errorf("decode user message: expected role %q, got %q", messageRoleUser, wire.Role)
	}
	content, err := unmarshalUserContent(wire.Content)
	if err != nil {
		return err
	}
	message.Content = content
	return nil
}

func (message AssistantMessage) MarshalJSON() ([]byte, error) {
	content, err := marshalAssistantContent(message.Content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(assistantMessageWire{
		Role:          messageRoleAssistant,
		Content:       content,
		Protocol:      message.Protocol,
		Provider:      message.Provider,
		Model:         message.Model,
		ResponseModel: message.ResponseModel,
		ResponseID:    message.ResponseID,
		Usage:         message.Usage,
		StopReason:    message.StopReason,
		ErrorMessage:  message.ErrorMessage,
		Diagnostics:   message.Diagnostics,
		Timestamp:     message.Timestamp,
	})
}

func (message *AssistantMessage) UnmarshalJSON(data []byte) error {
	if message == nil {
		return errors.New("cannot unmarshal assistant message into nil receiver")
	}
	var wire assistantMessageWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode assistant message: %w", err)
	}
	if wire.Role != messageRoleAssistant {
		return fmt.Errorf("decode assistant message: expected role %q, got %q", messageRoleAssistant, wire.Role)
	}
	content, err := unmarshalAssistantContent(wire.Content)
	if err != nil {
		return err
	}
	*message = AssistantMessage{
		Content:       content,
		Protocol:      wire.Protocol,
		Provider:      wire.Provider,
		Model:         wire.Model,
		ResponseModel: wire.ResponseModel,
		ResponseID:    wire.ResponseID,
		Usage:         wire.Usage,
		StopReason:    wire.StopReason,
		ErrorMessage:  wire.ErrorMessage,
		Diagnostics:   wire.Diagnostics,
		Timestamp:     wire.Timestamp,
	}
	return nil
}

func (message ToolResultMessage) MarshalJSON() ([]byte, error) {
	content, err := marshalToolResultContent(message.Content)
	if err != nil {
		return nil, err
	}
	return json.Marshal(toolResultMessageWire{
		Role:       messageRoleTool,
		ToolCallID: message.ToolCallID,
		ToolName:   message.ToolName,
		Content:    content,
		IsError:    message.IsError,
	})
}

func (message *ToolResultMessage) UnmarshalJSON(data []byte) error {
	if message == nil {
		return errors.New("cannot unmarshal tool result message into nil receiver")
	}
	var wire toolResultMessageWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode tool result message: %w", err)
	}
	if wire.Role != messageRoleTool {
		return fmt.Errorf("decode tool result message: expected role %q, got %q", messageRoleTool, wire.Role)
	}
	content, err := unmarshalToolResultContent(wire.Content)
	if err != nil {
		return err
	}
	*message = ToolResultMessage{
		ToolCallID: wire.ToolCallID,
		ToolName:   wire.ToolName,
		Content:    content,
		IsError:    wire.IsError,
	}
	return nil
}

func (input Context) MarshalJSON() ([]byte, error) {
	messages := make([]json.RawMessage, len(input.Messages))
	for index, message := range input.Messages {
		encoded, err := marshalMessage(message)
		if err != nil {
			return nil, fmt.Errorf("encode message %d: %w", index, err)
		}
		messages[index] = encoded
	}
	return json.Marshal(contextWire{
		SystemPrompt: input.SystemPrompt,
		Messages:     messages,
		Tools:        input.Tools,
	})
}

func (input *Context) UnmarshalJSON(data []byte) error {
	if input == nil {
		return errors.New("cannot unmarshal context into nil receiver")
	}
	var wire contextWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode context: %w", err)
	}
	messages := make([]Message, len(wire.Messages))
	for index, raw := range wire.Messages {
		message, err := unmarshalMessage(raw)
		if err != nil {
			return fmt.Errorf("decode message %d: %w", index, err)
		}
		messages[index] = message
	}
	*input = Context{
		SystemPrompt: wire.SystemPrompt,
		Messages:     messages,
		Tools:        wire.Tools,
	}
	return nil
}

// MarshalMessage encodes a single message to JSON, tagged with its role so
// UnmarshalMessage can decode it back to the right concrete type. It is the
// per-message counterpart to Context's JSON round-tripping, for persisting
// conversation history one message at a time (e.g. as JSON Lines).
func MarshalMessage(message Message) ([]byte, error) {
	return marshalMessage(message)
}

// UnmarshalMessage decodes a message produced by MarshalMessage, dispatching on
// the role tag to the concrete UserMessage, AssistantMessage, or
// ToolResultMessage.
func UnmarshalMessage(data []byte) (Message, error) {
	return unmarshalMessage(data)
}

func marshalMessage(message Message) (json.RawMessage, error) {
	switch typed := message.(type) {
	case *UserMessage:
		if typed == nil {
			return nil, errors.New("user message is nil")
		}
		return json.Marshal(typed)
	case *AssistantMessage:
		if typed == nil {
			return nil, errors.New("assistant message is nil")
		}
		return json.Marshal(typed)
	case *ToolResultMessage:
		if typed == nil {
			return nil, errors.New("tool result message is nil")
		}
		return json.Marshal(typed)
	default:
		return nil, fmt.Errorf("unsupported message type %T", message)
	}
}

func unmarshalMessage(data json.RawMessage) (Message, error) {
	if isJSONNull(data) {
		return nil, errors.New("message is null")
	}
	var header messageHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("decode message header: %w", err)
	}
	switch header.Role {
	case messageRoleUser:
		var message UserMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, err
		}
		return &message, nil
	case messageRoleAssistant:
		var message AssistantMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, err
		}
		return &message, nil
	case messageRoleTool:
		var message ToolResultMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, err
		}
		return &message, nil
	case "":
		return nil, errors.New("message role is missing")
	default:
		return nil, fmt.Errorf("unknown message role %q", header.Role)
	}
}

func marshalUserContent(content []UserContent) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(content))
	for index, item := range content {
		encoded, err := marshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("encode user content %d: %w", index, err)
		}
		result[index] = encoded
	}
	return result, nil
}

func marshalAssistantContent(content []AssistantContent) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(content))
	for index, item := range content {
		encoded, err := marshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("encode assistant content %d: %w", index, err)
		}
		result[index] = encoded
	}
	return result, nil
}

func marshalToolResultContent(content []ToolResultContent) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(content))
	for index, item := range content {
		encoded, err := marshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("encode tool result content %d: %w", index, err)
		}
		result[index] = encoded
	}
	return result, nil
}

func marshalContent(content any) (json.RawMessage, error) {
	switch typed := content.(type) {
	case *TextContent:
		if typed == nil {
			return nil, errors.New("text content is nil")
		}
		return json.Marshal(typed)
	case *ThinkingContent:
		if typed == nil {
			return nil, errors.New("thinking content is nil")
		}
		return json.Marshal(typed)
	case *ImageContent:
		if typed == nil {
			return nil, errors.New("image content is nil")
		}
		return json.Marshal(typed)
	case *ToolCall:
		if typed == nil {
			return nil, errors.New("tool call content is nil")
		}
		return json.Marshal(typed)
	default:
		return nil, fmt.Errorf("unsupported content type %T", content)
	}
}

func unmarshalUserContent(raw []json.RawMessage) ([]UserContent, error) {
	result := make([]UserContent, len(raw))
	for index, item := range raw {
		content, err := unmarshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("decode user content %d: %w", index, err)
		}
		switch typed := content.(type) {
		case *TextContent:
			result[index] = typed
		case *ImageContent:
			result[index] = typed
		default:
			return nil, fmt.Errorf("decode user content %d: content type %T is not allowed", index, content)
		}
	}
	return result, nil
}

func unmarshalAssistantContent(raw []json.RawMessage) ([]AssistantContent, error) {
	result := make([]AssistantContent, len(raw))
	for index, item := range raw {
		content, err := unmarshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("decode assistant content %d: %w", index, err)
		}
		switch typed := content.(type) {
		case *TextContent:
			result[index] = typed
		case *ThinkingContent:
			result[index] = typed
		case *ToolCall:
			result[index] = typed
		default:
			return nil, fmt.Errorf("decode assistant content %d: content type %T is not allowed", index, content)
		}
	}
	return result, nil
}

func unmarshalToolResultContent(raw []json.RawMessage) ([]ToolResultContent, error) {
	result := make([]ToolResultContent, len(raw))
	for index, item := range raw {
		content, err := unmarshalContent(item)
		if err != nil {
			return nil, fmt.Errorf("decode tool result content %d: %w", index, err)
		}
		switch typed := content.(type) {
		case *TextContent:
			result[index] = typed
		case *ImageContent:
			result[index] = typed
		default:
			return nil, fmt.Errorf("decode tool result content %d: content type %T is not allowed", index, content)
		}
	}
	return result, nil
}

func unmarshalContent(data json.RawMessage) (any, error) {
	if isJSONNull(data) {
		return nil, errors.New("content is null")
	}
	var header contentHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("decode content header: %w", err)
	}
	switch header.Type {
	case contentTypeText:
		var wire textContentWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, fmt.Errorf("decode text content: %w", err)
		}
		return &TextContent{Text: wire.Text, TextSignature: wire.TextSignature}, nil
	case contentTypeThinking:
		var wire thinkingContentWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, fmt.Errorf("decode thinking content: %w", err)
		}
		return &ThinkingContent{
			Thinking:          wire.Thinking,
			ThinkingSignature: wire.ThinkingSignature,
			Redacted:          wire.Redacted,
		}, nil
	case contentTypeImage:
		var wire imageContentWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, fmt.Errorf("decode image content: %w", err)
		}
		return &ImageContent{Data: wire.Data, MIMEType: wire.MIMEType}, nil
	case contentTypeToolCall:
		var wire toolCallWire
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, fmt.Errorf("decode tool call content: %w", err)
		}
		arguments := wire.Arguments
		if arguments == nil {
			arguments = make(map[string]any)
		}
		return &ToolCall{
			ID:               wire.ID,
			Name:             wire.Name,
			Arguments:        arguments,
			ThoughtSignature: wire.ThoughtSignature,
		}, nil
	case "":
		return nil, errors.New("content type is missing")
	default:
		return nil, fmt.Errorf("unknown content type %q", header.Type)
	}
}
