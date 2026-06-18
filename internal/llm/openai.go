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
// package events. This initial implementation supports text messages only.
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

		stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Model:    model.ID,
			Messages: messages,
		})
		defer stream.Close()

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
		text, err := messageText(message)
		if err != nil {
			return nil, err
		}

		switch message.Role {
		case RoleUser:
			messages = append(messages, openai.UserMessage(text))
		case RoleAssistant:
			messages = append(messages, openai.AssistantMessage(text))
		case RoleToolResult:
			if message.ToolCallID == "" {
				return nil, errors.New("tool result message is missing tool call ID")
			}
			messages = append(messages, openai.ToolMessage(text, message.ToolCallID))
		default:
			return nil, fmt.Errorf("unsupported message role %q", message.Role)
		}
	}

	return messages, nil
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
	clone.Content = append([]Content(nil), message.Content...)
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
