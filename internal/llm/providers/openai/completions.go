package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Adapter translates the OpenAI-compatible Chat Completions protocol.
type Adapter struct {
	httpClient *http.Client
}

// NewAdapter creates an adapter that uses httpClient for requests.
// A nil client uses http.DefaultClient.
func NewAdapter(httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Adapter{httpClient: httpClient}
}

// Protocol returns the registry key for the Chat Completions protocol.
func (a *Adapter) Protocol() llm.Protocol {
	return llm.ProtocolOpenAICompletions
}

// Stream starts a Chat Completions request and translates SDK chunks into
// package events. It supports text, reasoning, and tool call content.
func (a *Adapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	if model.Protocol != a.Protocol() {
		return nil, fmt.Errorf("model protocol %q does not match adapter protocol %q", model.Protocol, a.Protocol())
	}
	if model.ID == "" {
		return nil, errors.New("model ID is empty")
	}
	if options.APIKey == "" {
		return nil, errors.New("OpenAI API key is empty")
	}

	messages, err := convertMessages(input)
	if err != nil {
		return nil, err
	}

	tools, err := convertTools(input.Tools)
	if err != nil {
		return nil, err
	}

	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
		option.WithHTTPClient(a.httpClient),
	}
	if model.BaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(model.BaseURL))
	}
	client := oai.NewClient(clientOptions...)

	events := make(chan llm.Event)
	go func() {
		defer close(events)

		output := llm.NewAssistantMessage(model)
		events <- llm.Event{Type: llm.EventStart, Partial: cloneAssistantMessage(output)}

		params := oai.ChatCompletionNewParams{
			Model:    model.ID,
			Messages: messages,
			StreamOptions: oai.ChatCompletionStreamOptionsParam{
				IncludeUsage: oai.Bool(true),
			},
		}
		if len(tools) > 0 {
			params.Tools = tools
		}
		stream := client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

		toolCallsByIndex := make(map[int64]*llm.ToolCall)
		finishReason := ""
		for stream.Next() {
			chunk := stream.Current()
			if output.ResponseID == "" {
				output.ResponseID = chunk.ID
			}
			if output.ResponseModel == "" && chunk.Model != "" && chunk.Model != model.ID {
				output.ResponseModel = chunk.Model
			}
			if chunk.JSON.Usage.Valid() {
				output.Usage = parseUsage(chunk)
			}
			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			reasoningDelta, err := extraString(choice.Delta.JSON.ExtraFields, "reasoning_content")
			if err != nil {
				emitError(events, output, ctx, err)
				return
			}
			if reasoningDelta != "" {
				content, contentIndex, started := ensureAssistantThinking(&output)
				if started {
					events <- llm.Event{
						Type:         llm.EventThinkingStart,
						ContentIndex: contentIndex,
						Partial:      cloneAssistantMessage(output),
					}
				}
				content.Thinking += reasoningDelta
				events <- llm.Event{
					Type:         llm.EventThinkingDelta,
					ContentIndex: contentIndex,
					Delta:        reasoningDelta,
					Partial:      cloneAssistantMessage(output),
				}
			}
			if choice.Delta.Content != "" {
				content, contentIndex, started := ensureAssistantText(&output)
				if started {
					events <- llm.Event{
						Type:         llm.EventTextStart,
						ContentIndex: contentIndex,
						Partial:      cloneAssistantMessage(output),
					}
				}
				content.Text += choice.Delta.Content
				events <- llm.Event{
					Type:         llm.EventTextDelta,
					ContentIndex: contentIndex,
					Delta:        choice.Delta.Content,
					Partial:      cloneAssistantMessage(output),
				}
			}
			for _, toolDelta := range choice.Delta.ToolCalls {
				block, contentIndex, started := ensureAssistantToolCall(&output, toolCallsByIndex, toolDelta)
				if started {
					events <- llm.Event{
						Type:         llm.EventToolCallStart,
						ContentIndex: contentIndex,
						ToolCall:     cloneToolCall(block),
						Partial:      cloneAssistantMessage(output),
					}
				}
				if toolDelta.Function.Arguments != "" {
					block.Arguments += toolDelta.Function.Arguments
				}
				events <- llm.Event{
					Type:         llm.EventToolCallDelta,
					ContentIndex: contentIndex,
					Delta:        toolDelta.Function.Arguments,
					ToolCall:     cloneToolCall(block),
					Partial:      cloneAssistantMessage(output),
				}
			}
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
		}

		if err := stream.Err(); err != nil {
			emitError(events, output, ctx, err)
			return
		}

		stopReason, err := mapStopReason(finishReason)
		if err != nil {
			emitError(events, output, ctx, err)
			return
		}
		output.StopReason = stopReason
		for contentIndex, rawContent := range output.Content {
			switch content := rawContent.(type) {
			case *llm.TextContent:
				events <- llm.Event{
					Type:         llm.EventTextEnd,
					ContentIndex: contentIndex,
					Content:      content.Text,
					Partial:      cloneAssistantMessage(output),
				}
			case *llm.ThinkingContent:
				events <- llm.Event{
					Type:         llm.EventThinkingEnd,
					ContentIndex: contentIndex,
					Content:      content.Thinking,
					Partial:      cloneAssistantMessage(output),
				}
			case *llm.ToolCall:
				events <- llm.Event{
					Type:         llm.EventToolCallEnd,
					ContentIndex: contentIndex,
					ToolCall:     cloneToolCall(content),
					Partial:      cloneAssistantMessage(output),
				}
			}
		}
		events <- llm.Event{Type: llm.EventDone, Message: cloneAssistantMessage(output)}
	}()

	return events, nil
}
