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

func convertMessages(
	input llm.Context,
	model llm.Model,
	compat resolvedCompat,
) ([]oai.ChatCompletionMessageParamUnion, error) {
	transformed := llm.TransformMessages(input.Messages, model, toolCallIDNormalizer(model))
	messages := make([]oai.ChatCompletionMessageParamUnion, 0, len(transformed)+1)
	if input.SystemPrompt != "" {
		if model.Reasoning && compat.supportsDeveloperRole {
			messages = append(messages, oai.DeveloperMessage(input.SystemPrompt))
		} else {
			messages = append(messages, oai.SystemMessage(input.SystemPrompt))
		}
	}

	for i := 0; i < len(transformed); i++ {
		rawMessage := transformed[i]
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
			assistant, err := convertAssistantMessage(message, model, compat)
			if err != nil {
				return nil, err
			}
			if assistant != nil {
				messages = append(messages, oai.ChatCompletionMessageParamUnion{OfAssistant: assistant})
			}
		case *llm.ToolResultMessage:
			var images []oai.ChatCompletionContentPartUnionParam
			for ; i < len(transformed); i++ {
				toolResult, ok := transformed[i].(*llm.ToolResultMessage)
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

// toolCallIDNormalizer returns the rewrite applied to tool-call IDs when an
// assistant turn from another model is replayed against this one. It collapses
// the pipe-separated IDs the OpenAI Responses API emits ({call_id}|{long_id})
// into a sanitized, length-limited call_id, and truncates over-long IDs for the
// native OpenAI endpoint. Other providers keep their IDs unchanged.
func toolCallIDNormalizer(model llm.Model) func(string) string {
	return func(id string) string {
		if index := strings.IndexByte(id, '|'); index >= 0 {
			return truncateASCII(sanitizeToolCallID(id[:index]), 40)
		}
		if model.Provider == "openai" {
			return truncateASCII(id, 40)
		}
		return id
	}
}

// sanitizeToolCallID replaces any character outside the Anthropic-compatible set
// [a-zA-Z0-9_-] with an underscore.
func sanitizeToolCallID(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, id)
}

// truncateASCII caps an ASCII identifier at limit bytes. Tool-call IDs are ASCII,
// so a byte slice never splits a multi-byte rune.
func truncateASCII(id string, limit int) string {
	if len(id) > limit {
		return id[:limit]
	}
	return id
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
func convertAssistantMessage(
	message *llm.AssistantMessage,
	model llm.Model,
	compat resolvedCompat,
) (*oai.ChatCompletionAssistantMessageParam, error) {
	assistant := &oai.ChatCompletionAssistantMessageParam{}
	var text strings.Builder
	var reasoning strings.Builder
	reasoningSig := ""
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
			// Skip empty reasoning and replay only what carries content. The first
			// non-empty block's signature names the field this provider expects.
			if strings.TrimSpace(content.Thinking) == "" {
				continue
			}
			if reasoning.Len() > 0 {
				reasoning.WriteString("\n")
			}
			if reasoningSig == "" {
				reasoningSig = content.ThinkingSignature
			}
			reasoning.WriteString(content.Thinking)
		case *llm.ToolCall:
			if content == nil {
				return nil, errors.New("assistant tool call content is missing tool call data")
			}
			arguments, err := encodeToolArguments(content.Arguments)
			if err != nil {
				return nil, fmt.Errorf("encode arguments for tool call %q: %w", content.Name, err)
			}
			assistant.ToolCalls = append(assistant.ToolCalls, oai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &oai.ChatCompletionMessageFunctionToolCallParam{
					ID: content.ID,
					Function: oai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      content.Name,
						Arguments: arguments,
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
	extras := map[string]any{}
	// Replay reasoning only when its source field is known: a provider rejects
	// reasoning sent under a field it does not expect, so unsigned reasoning is
	// dropped rather than guessed.
	if field := reasoningSignature(model, reasoningSig); field != "" {
		if reasoningValue := reasoning.String(); reasoningValue != "" {
			extras[field] = reasoningValue
		}
	}
	// Some providers require every replayed assistant message to carry
	// reasoning_content once reasoning is enabled, even when it is empty.
	if compat.requiresReasoningContentOnAssistantMessages && model.Reasoning {
		if _, ok := extras["reasoning_content"]; !ok {
			extras["reasoning_content"] = ""
		}
	}
	if len(extras) > 0 {
		assistant.SetExtraFields(extras)
	}
	if !hasText && len(assistant.ToolCalls) == 0 {
		return nil, nil
	}
	return assistant, nil
}

func encodeToolArguments(arguments map[string]any) (string, error) {
	if arguments == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// convertTools maps tool definitions to OpenAI function tool params.
func convertTools(tools []llm.ToolDefinition, compat resolvedCompat) ([]oai.ChatCompletionToolUnionParam, error) {
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
		if compat.supportsStrictMode {
			// Match pi: advertise the standard strict field while leaving strict
			// schema enforcement disabled unless the public Tool API gains an
			// explicit strict option.
			function.Strict = oai.Bool(false)
		}

		converted = append(converted, oai.ChatCompletionFunctionTool(function))
	}

	return converted, nil
}
