package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
)

const OpenAICompletionsAPI = "openai-completions"

// OpenAIProvider adapts the OpenAI-compatible Chat Completions API.
type OpenAIProvider struct {
	httpClient *http.Client
}

// NewOpenAIProvider creates a provider that uses httpClient for requests.
// A nil client uses http.DefaultClient.
func NewOpenAIProvider(httpClient *http.Client) *OpenAIProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &OpenAIProvider{httpClient: httpClient}
}

// API returns the registry key for the Chat Completions protocol.
func (p *OpenAIProvider) API() string {
	return OpenAICompletionsAPI
}

// Stream starts a Chat Completions request and translates SDK chunks into
// package events. It supports text, reasoning, and tool call content.
func (p *OpenAIProvider) Stream(
	ctx context.Context,
	model Model,
	input Context,
	options StreamOptions,
) (<-chan Event, error) {
	if model.API != p.API() {
		return nil, fmt.Errorf("model API %q does not match provider API %q", model.API, p.API())
	}
	if model.ID == "" {
		return nil, errors.New("model ID is empty")
	}
	if options.APIKey == "" {
		return nil, errors.New("OpenAI API key is empty")
	}

	messages, err := convertOpenAIMessages(input)
	if err != nil {
		return nil, err
	}

	tools, err := convertOpenAITools(input.Tools)
	if err != nil {
		return nil, err
	}

	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
		option.WithHTTPClient(p.httpClient),
	}
	if model.BaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(model.BaseURL))
	}
	client := openai.NewClient(clientOptions...)

	events := make(chan Event)
	go func() {
		defer close(events)

		output := AssistantMessage{Model: model.ID}
		events <- Event{Type: EventStart, Partial: cloneAssistantMessage(output)}

		params := openai.ChatCompletionNewParams{
			Model:    model.ID,
			Messages: messages,
		}
		if len(tools) > 0 {
			params.Tools = tools
		}
		stream := client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

		toolCallsByIndex := make(map[int64]*ToolCall)
		finishReason := ""
		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			reasoningDelta, err := openAIExtraString(choice.Delta.JSON.ExtraFields, "reasoning_content")
			if err != nil {
				emitOpenAIError(events, output, ctx, err)
				return
			}
			if reasoningDelta != "" {
				appendAssistantThinking(&output, reasoningDelta)
				events <- Event{
					Type:    EventThinkingDelta,
					Delta:   reasoningDelta,
					Partial: cloneAssistantMessage(output),
				}
			}
			if choice.Delta.Content != "" {
				appendAssistantText(&output, choice.Delta.Content)
				events <- Event{
					Type:    EventTextDelta,
					Delta:   choice.Delta.Content,
					Partial: cloneAssistantMessage(output),
				}
			}
			for _, toolDelta := range choice.Delta.ToolCalls {
				block := ensureAssistantToolCall(&output, toolCallsByIndex, toolDelta)
				if toolDelta.Function.Arguments != "" {
					block.Arguments += toolDelta.Function.Arguments
				}
				events <- Event{
					Type:     EventToolCallDelta,
					Delta:    toolDelta.Function.Arguments,
					ToolCall: cloneToolCall(block),
					Partial:  cloneAssistantMessage(output),
				}
			}
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
		}

		if err := stream.Err(); err != nil {
			emitOpenAIError(events, output, ctx, err)
			return
		}

		stopReason, err := mapOpenAIStopReason(finishReason)
		if err != nil {
			emitOpenAIError(events, output, ctx, err)
			return
		}
		output.StopReason = stopReason
		for _, content := range output.Content {
			if content.Type == ContentToolCall && content.ToolCall != nil {
				events <- Event{
					Type:     EventToolCallEnd,
					ToolCall: cloneToolCall(content.ToolCall),
					Partial:  cloneAssistantMessage(output),
				}
			}
		}
		events <- Event{Type: EventDone, Message: cloneAssistantMessage(output)}
	}()

	return events, nil
}

func convertOpenAIMessages(input Context) ([]openai.ChatCompletionMessageParamUnion, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(input.Messages)+1)
	if input.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(input.SystemPrompt))
	}

	for _, message := range input.Messages {
		switch message.Role {
		case RoleUser:
			text, err := messageText(message)
			if err != nil {
				return nil, err
			}
			messages = append(messages, openai.UserMessage(text))
		case RoleAssistant:
			assistant, err := convertAssistantMessage(message)
			if err != nil {
				return nil, err
			}
			if assistant == nil {
				continue
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: assistant})
		case RoleToolResult:
			if message.ToolCallID == "" {
				return nil, errors.New("tool result message is missing tool call ID")
			}
			text, err := messageText(message)
			if err != nil {
				return nil, err
			}
			messages = append(messages, openai.ToolMessage(text, message.ToolCallID))
		default:
			return nil, fmt.Errorf("unsupported message role %q", message.Role)
		}
	}

	return messages, nil
}

// convertAssistantMessage serializes an assistant message, including any tool
// calls, into an OpenAI assistant message param. It returns nil for an empty
// message (no text and no tool calls), which the caller skips: some providers
// reject assistant messages that carry neither content nor tool calls.
func convertAssistantMessage(message Message) (*openai.ChatCompletionAssistantMessageParam, error) {
	assistant := &openai.ChatCompletionAssistantMessageParam{}
	var text strings.Builder
	var reasoning strings.Builder
	for _, content := range message.Content {
		switch content.Type {
		case ContentText:
			text.WriteString(content.Text)
		case ContentThinking:
			reasoning.WriteString(content.Thinking)
		case ContentToolCall:
			if content.ToolCall == nil {
				return nil, errors.New("assistant tool call content is missing tool call data")
			}
			assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: content.ToolCall.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      content.ToolCall.Name,
						Arguments: content.ToolCall.Arguments,
					},
				},
			})
		default:
			return nil, fmt.Errorf("content type %q is not supported in assistant messages", content.Type)
		}
	}

	hasText := false
	if value := text.String(); value != "" {
		assistant.Content.OfString = openai.String(value)
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

// convertOpenAITools maps tool definitions to OpenAI function tool params.
func convertOpenAITools(tools []ToolDefinition) ([]openai.ChatCompletionToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	converted := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			return nil, errors.New("tool definition is missing a name")
		}

		function := shared.FunctionDefinitionParam{Name: tool.Name}
		if tool.Description != "" {
			function.Description = openai.String(tool.Description)
		}
		if len(tool.Parameters) > 0 {
			var parameters map[string]any
			if err := json.Unmarshal(tool.Parameters, &parameters); err != nil {
				return nil, fmt.Errorf("decode parameters for tool %q: %w", tool.Name, err)
			}
			function.Parameters = parameters
		}

		converted = append(converted, openai.ChatCompletionFunctionTool(function))
	}

	return converted, nil
}

// ensureAssistantToolCall finds or creates the streaming tool call block for a
// delta's index, appending a new block to the message content on first sight and
// backfilling the id and name as they arrive across chunks.
func ensureAssistantToolCall(
	message *AssistantMessage,
	byIndex map[int64]*ToolCall,
	delta openai.ChatCompletionChunkChoiceDeltaToolCall,
) *ToolCall {
	block, ok := byIndex[delta.Index]
	if !ok {
		block = &ToolCall{ID: delta.ID, Name: delta.Function.Name}
		byIndex[delta.Index] = block
		message.Content = append(message.Content, Content{Type: ContentToolCall, ToolCall: block})
	}
	if block.ID == "" && delta.ID != "" {
		block.ID = delta.ID
	}
	if block.Name == "" && delta.Function.Name != "" {
		block.Name = delta.Function.Name
	}
	return block
}

func messageText(message Message) (string, error) {
	var text strings.Builder
	for _, content := range message.Content {
		switch content.Type {
		case ContentText:
			text.WriteString(content.Text)
		case ContentThinking:
			if message.Role != RoleAssistant {
				return "", fmt.Errorf("thinking content is not valid for role %q", message.Role)
			}
		default:
			return "", fmt.Errorf("content type %q is not supported by the text-only OpenAI provider", content.Type)
		}
	}
	return text.String(), nil
}

func appendAssistantText(message *AssistantMessage, delta string) {
	for i := range message.Content {
		if message.Content[i].Type == ContentText {
			message.Content[i].Text += delta
			return
		}
	}
	message.Content = append(message.Content, Content{Type: ContentText, Text: delta})
}

func appendAssistantThinking(message *AssistantMessage, delta string) {
	for i := range message.Content {
		if message.Content[i].Type == ContentThinking {
			message.Content[i].Thinking += delta
			return
		}
	}
	message.Content = append(message.Content, Content{Type: ContentThinking, Thinking: delta})
}

func cloneAssistantMessage(message AssistantMessage) *AssistantMessage {
	clone := message
	clone.Content = make([]Content, len(message.Content))
	for i, content := range message.Content {
		clone.Content[i] = content
		clone.Content[i].ToolCall = cloneToolCall(content.ToolCall)
	}
	return &clone
}

func cloneToolCall(toolCall *ToolCall) *ToolCall {
	if toolCall == nil {
		return nil
	}
	clone := *toolCall
	return &clone
}

func emitOpenAIError(events chan<- Event, output AssistantMessage, ctx context.Context, err error) {
	if ctx.Err() != nil {
		output.StopReason = "aborted"
		err = ctx.Err()
	} else {
		output.StopReason = "error"
	}
	events <- Event{Type: EventError, Message: cloneAssistantMessage(output), Err: err}
}

func openAIExtraString(fields map[string]respjson.Field, name string) (string, error) {
	field, ok := fields[name]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return "", nil
	}

	var value string
	if err := json.Unmarshal([]byte(field.Raw()), &value); err != nil {
		return "", fmt.Errorf("decode OpenAI %s field: %w", name, err)
	}
	return value, nil
}

func mapOpenAIStopReason(reason string) (string, error) {
	switch reason {
	case "stop":
		return "stop", nil
	case "length":
		return "length", nil
	case "tool_calls", "function_call":
		return "toolUse", nil
	case "content_filter":
		return "", errors.New("OpenAI response was blocked by the content filter")
	case "":
		return "", errors.New("OpenAI stream ended without a finish reason")
	default:
		return "", fmt.Errorf("unsupported OpenAI finish reason %q", reason)
	}
}
