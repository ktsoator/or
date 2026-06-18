package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ktsoator/or/internal/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/respjson"
)

// ensureAssistantToolCall finds or creates the streaming tool call block for a
// delta's index, appending a new block to the message content on first sight and
// backfilling the id and name as they arrive across chunks.
func ensureAssistantToolCall(
	message *llm.AssistantMessage,
	byIndex map[int64]*llm.ToolCall,
	delta oai.ChatCompletionChunkChoiceDeltaToolCall,
) (*llm.ToolCall, int, bool) {
	block, ok := byIndex[delta.Index]
	started := !ok
	if !ok {
		block = &llm.ToolCall{ID: delta.ID, Name: delta.Function.Name}
		byIndex[delta.Index] = block
		message.Content = append(message.Content, block)
	}
	if block.ID == "" && delta.ID != "" {
		block.ID = delta.ID
	}
	if block.Name == "" && delta.Function.Name != "" {
		block.Name = delta.Function.Name
	}
	return block, assistantContentIndex(message.Content, block), started
}

func ensureAssistantText(message *llm.AssistantMessage) (*llm.TextContent, int, bool) {
	for i, rawContent := range message.Content {
		if content, ok := rawContent.(*llm.TextContent); ok && content != nil {
			return content, i, false
		}
	}
	content := &llm.TextContent{}
	message.Content = append(message.Content, content)
	return content, len(message.Content) - 1, true
}

func ensureAssistantThinking(message *llm.AssistantMessage) (*llm.ThinkingContent, int, bool) {
	for i, rawContent := range message.Content {
		if content, ok := rawContent.(*llm.ThinkingContent); ok && content != nil {
			return content, i, false
		}
	}
	content := &llm.ThinkingContent{}
	message.Content = append(message.Content, content)
	return content, len(message.Content) - 1, true
}

func assistantContentIndex(content []llm.AssistantContent, target llm.AssistantContent) int {
	for i, candidate := range content {
		if candidate == target {
			return i
		}
	}
	return -1
}

func cloneAssistantMessage(message llm.AssistantMessage) *llm.AssistantMessage {
	clone := message
	clone.Content = make([]llm.AssistantContent, len(message.Content))
	for i, rawContent := range message.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			if content != nil {
				copied := *content
				clone.Content[i] = &copied
			}
		case *llm.ThinkingContent:
			if content != nil {
				copied := *content
				clone.Content[i] = &copied
			}
		case *llm.ToolCall:
			clone.Content[i] = cloneToolCall(content)
		}
	}
	return &clone
}

func cloneToolCall(toolCall *llm.ToolCall) *llm.ToolCall {
	if toolCall == nil {
		return nil
	}
	clone := *toolCall
	return &clone
}

func emitError(events chan<- llm.Event, output llm.AssistantMessage, ctx context.Context, err error) {
	if ctx.Err() != nil {
		output.StopReason = "aborted"
		err = ctx.Err()
	} else {
		output.StopReason = "error"
	}
	output.ErrorMessage = err.Error()
	events <- llm.Event{Type: llm.EventError, Message: cloneAssistantMessage(output), Err: err}
}

func parseUsage(chunk oai.ChatCompletionChunk) llm.Usage {
	usage := chunk.Usage
	cacheRead := usage.PromptTokensDetails.CachedTokens
	input := max(0, usage.PromptTokens-cacheRead)
	return llm.Usage{
		Input:       input,
		Output:      usage.CompletionTokens,
		CacheRead:   cacheRead,
		TotalTokens: input + usage.CompletionTokens + cacheRead,
	}
}

func extraString(fields map[string]respjson.Field, name string) (string, error) {
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

func mapStopReason(reason string) (string, error) {
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
