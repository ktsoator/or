package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func convertMessages(input llm.Context) ([]oai.ChatCompletionMessageParamUnion, error) {
	messages := make([]oai.ChatCompletionMessageParamUnion, 0, len(input.Messages)+1)
	if input.SystemPrompt != "" {
		messages = append(messages, oai.SystemMessage(input.SystemPrompt))
	}

	for i := 0; i < len(input.Messages); i++ {
		rawMessage := input.Messages[i]
		switch message := rawMessage.(type) {
		case *llm.UserMessage:
			if message == nil {
				return nil, errors.New("user message is nil")
			}
			userMessage, err := convertUserMessage(message)
			if err != nil {
				return nil, err
			}
			if userMessage != nil {
				messages = append(messages, *userMessage)
			}
		case *llm.AssistantMessage:
			if message == nil {
				return nil, errors.New("assistant message is nil")
			}
			assistant, err := convertAssistantMessage(message)
			if err != nil {
				return nil, err
			}
			if assistant != nil {
				messages = append(messages, oai.ChatCompletionMessageParamUnion{OfAssistant: assistant})
			}
		case *llm.ToolResultMessage:
			var images []oai.ChatCompletionContentPartUnionParam
			for ; i < len(input.Messages); i++ {
				toolResult, ok := input.Messages[i].(*llm.ToolResultMessage)
				if !ok {
					break
				}
				toolMessage, resultImages, err := convertToolResultMessage(toolResult)
				if err != nil {
					return nil, err
				}
				messages = append(messages, toolMessage)
				images = append(images, resultImages...)
			}
			i--
			if len(images) > 0 {
				parts := []oai.ChatCompletionContentPartUnionParam{
					oai.TextContentPart("Attached image(s) from tool result:"),
				}
				parts = append(parts, images...)
				messages = append(messages, oai.UserMessage(parts))
			}
		default:
			return nil, fmt.Errorf("unsupported message type %T", rawMessage)
		}
	}

	return messages, nil
}

func convertUserMessage(message *llm.UserMessage) (*oai.ChatCompletionMessageParamUnion, error) {
	// OpenAI accepts a plain string for a user message containing only text.
	// Keep that simpler representation instead of wrapping it in content parts.
	if len(message.Content) == 1 {
		if content, ok := message.Content[0].(*llm.TextContent); ok && content != nil {
			converted := oai.UserMessage(content.Text)
			return &converted, nil
		}
	}

	// Multiple text blocks and images use OpenAI's multipart content format.
	parts := make([]oai.ChatCompletionContentPartUnionParam, 0, len(message.Content))
	for _, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			if content == nil {
				return nil, errors.New("user text content is nil")
			}
			parts = append(parts, oai.TextContentPart(content.Text))
		case *llm.ImageContent:
			image, err := convertImageContent(content)
			if err != nil {
				return nil, err
			}
			parts = append(parts, image)
		default:
			return nil, fmt.Errorf("unsupported user content type %T", rawContent)
		}
	}
	// An empty internal message is skipped by convertMessages.
	if len(parts) == 0 {
		return nil, nil
	}

	converted := oai.UserMessage(parts)
	return &converted, nil
}

func convertToolResultMessage(message *llm.ToolResultMessage) (
	oai.ChatCompletionMessageParamUnion,
	[]oai.ChatCompletionContentPartUnionParam,
	error,
) {
	if message == nil {
		return oai.ChatCompletionMessageParamUnion{}, nil, errors.New("tool result message is nil")
	}
	if message.ToolCallID == "" {
		return oai.ChatCompletionMessageParamUnion{}, nil, errors.New("tool result message is missing tool call ID")
	}

	var textParts []string
	var images []oai.ChatCompletionContentPartUnionParam
	for _, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			if content == nil {
				return oai.ChatCompletionMessageParamUnion{}, nil, errors.New("tool result text content is nil")
			}
			textParts = append(textParts, content.Text)
		case *llm.ImageContent:
			image, err := convertImageContent(content)
			if err != nil {
				return oai.ChatCompletionMessageParamUnion{}, nil, err
			}
			images = append(images, image)
		default:
			return oai.ChatCompletionMessageParamUnion{}, nil,
				fmt.Errorf("unsupported tool result content type %T", rawContent)
		}
	}

	result := strings.Join(textParts, "\n")
	if result == "" && len(images) > 0 {
		result = "(see attached image)"
	}
	return oai.ToolMessage(result, message.ToolCallID), images, nil
}

func convertImageContent(content *llm.ImageContent) (oai.ChatCompletionContentPartUnionParam, error) {
	if content == nil {
		return oai.ChatCompletionContentPartUnionParam{}, errors.New("image content is nil")
	}
	if content.MIMEType == "" {
		return oai.ChatCompletionContentPartUnionParam{}, errors.New("image content is missing MIME type")
	}
	if content.Data == "" {
		return oai.ChatCompletionContentPartUnionParam{}, errors.New("image content is missing data")
	}
	return oai.ImageContentPart(oai.ChatCompletionContentPartImageImageURLParam{
		URL: "data:" + content.MIMEType + ";base64," + content.Data,
	}), nil
}

// convertAssistantMessage serializes an assistant message, including any tool
// calls, into an OpenAI assistant message param. It returns nil for an empty
// message (no text and no tool calls), which the caller skips: some providers
// reject assistant messages that carry neither content nor tool calls.
func convertAssistantMessage(message *llm.AssistantMessage) (*oai.ChatCompletionAssistantMessageParam, error) {
	assistant := &oai.ChatCompletionAssistantMessageParam{}
	var text strings.Builder
	var reasoning strings.Builder
	for _, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			if content == nil {
				return nil, errors.New("assistant text content is nil")
			}
			text.WriteString(content.Text)
		case *llm.ThinkingContent:
			if content == nil {
				return nil, errors.New("assistant thinking content is nil")
			}
			reasoning.WriteString(content.Thinking)
		case *llm.ToolCall:
			if content == nil {
				return nil, errors.New("assistant tool call content is missing tool call data")
			}
			assistant.ToolCalls = append(assistant.ToolCalls, oai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &oai.ChatCompletionMessageFunctionToolCallParam{
					ID: content.ID,
					Function: oai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      content.Name,
						Arguments: content.Arguments,
					},
				},
			})
		default:
			return nil, fmt.Errorf("unsupported assistant content type %T", rawContent)
		}
	}

	hasText := false
	if value := text.String(); value != "" {
		assistant.Content.OfString = oai.String(value)
		hasText = true
	}
	if value := reasoning.String(); value != "" {
		assistant.SetExtraFields(map[string]any{
			"reasoning_content": value,
		})
	}
	if !hasText && len(assistant.ToolCalls) == 0 {
		return nil, nil
	}
	return assistant, nil
}

// convertTools maps tool definitions to OpenAI function tool params.
func convertTools(tools []llm.ToolDefinition) ([]oai.ChatCompletionToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	converted := make([]oai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			return nil, errors.New("tool definition is missing a name")
		}

		function := shared.FunctionDefinitionParam{Name: tool.Name}
		if tool.Description != "" {
			function.Description = oai.String(tool.Description)
		}
		if len(tool.Parameters) > 0 {
			var parameters map[string]any
			if err := json.Unmarshal(tool.Parameters, &parameters); err != nil {
				return nil, fmt.Errorf("decode parameters for tool %q: %w", tool.Name, err)
			}
			function.Parameters = parameters
		}

		converted = append(converted, oai.ChatCompletionFunctionTool(function))
	}

	return converted, nil
}
