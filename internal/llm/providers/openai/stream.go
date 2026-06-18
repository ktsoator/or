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

// streamState owns all mutable data accumulated while consuming one provider
// response. Keeping it separate from Adapter.Stream makes the public entry point
// responsible only for validation and request construction.
type streamState struct {
	model  llm.Model
	output llm.AssistantMessage

	toolCallsByIndex map[int64]*llm.ToolCall
	toolCallsByID    map[string]*llm.ToolCall
	toolArgumentJSON map[*llm.ToolCall]string

	finishReason string
	started      bool
}

func newStreamState(model llm.Model) *streamState {
	return &streamState{
		model:            model,
		output:           llm.NewAssistantMessage(model),
		toolCallsByIndex: make(map[int64]*llm.ToolCall),
		toolCallsByID:    make(map[string]*llm.ToolCall),
		toolArgumentJSON: make(map[*llm.ToolCall]string),
	}
}

// consumeStream owns the SDK stream and guarantees exactly one terminal event
// before closing events: done for a successful response or error for a failure.
func consumeStream(
	ctx context.Context,
	client oai.Client,
	params oai.ChatCompletionNewParams,
	model llm.Model,
	events chan<- llm.Event,
) {
	defer close(events)

	state := newStreamState(model)
	stream := client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	for stream.Next() {
		if !state.started {
			state.started = true
			events <- llm.Event{Type: llm.EventStart, Partial: cloneAssistantMessage(state.output)}
		}
		if err := state.processChunk(stream.Current(), events); err != nil {
			emitError(events, state.output, ctx, err)
			return
		}
	}

	if err := stream.Err(); err != nil {
		emitError(events, state.output, ctx, err)
		return
	}
	if err := state.finish(events); err != nil {
		emitError(events, state.output, ctx, err)
	}
}

// processChunk merges one provider chunk into the partial assistant message and
// emits the corresponding delta events.
func (state *streamState) processChunk(chunk oai.ChatCompletionChunk, events chan<- llm.Event) error {
	if state.output.ResponseID == "" {
		state.output.ResponseID = chunk.ID
	}
	if state.output.ResponseModel == "" && chunk.Model != "" && chunk.Model != state.model.ID {
		state.output.ResponseModel = chunk.Model
	}
	if chunk.JSON.Usage.Valid() {
		state.output.Usage = usageFrom(chunk.Usage, state.model)
	}
	if len(chunk.Choices) == 0 {
		return nil
	}

	choice := chunk.Choices[0]
	// Moonshot and a few compatible providers report usage inside choice rather
	// than in the standard top-level chunk field.
	if !chunk.JSON.Usage.Valid() {
		usage, ok, err := usageFromExtra(choice.JSON.ExtraFields, "usage", state.model)
		if err != nil {
			return err
		}
		if ok {
			state.output.Usage = usage
		}
	}

	reasoningDelta, err := extraReasoning(choice.Delta.JSON.ExtraFields)
	if err != nil {
		return err
	}
	if reasoningDelta != "" {
		content, contentIndex, started := ensureAssistantThinking(&state.output)
		if started {
			events <- llm.Event{
				Type:         llm.EventThinkingStart,
				ContentIndex: contentIndex,
				Partial:      cloneAssistantMessage(state.output),
			}
		}
		content.Thinking += reasoningDelta
		events <- llm.Event{
			Type:         llm.EventThinkingDelta,
			ContentIndex: contentIndex,
			Delta:        reasoningDelta,
			Partial:      cloneAssistantMessage(state.output),
		}
	}

	if choice.Delta.Content != "" {
		content, contentIndex, started := ensureAssistantText(&state.output)
		if started {
			events <- llm.Event{
				Type:         llm.EventTextStart,
				ContentIndex: contentIndex,
				Partial:      cloneAssistantMessage(state.output),
			}
		}
		content.Text += choice.Delta.Content
		events <- llm.Event{
			Type:         llm.EventTextDelta,
			ContentIndex: contentIndex,
			Delta:        choice.Delta.Content,
			Partial:      cloneAssistantMessage(state.output),
		}
	}

	for _, toolDelta := range choice.Delta.ToolCalls {
		block, contentIndex, started := ensureAssistantToolCall(
			&state.output,
			state.toolCallsByIndex,
			state.toolCallsByID,
			toolDelta,
		)
		if started {
			events <- llm.Event{
				Type:         llm.EventToolCallStart,
				ContentIndex: contentIndex,
				ToolCall:     cloneToolCall(block),
				Partial:      cloneAssistantMessage(state.output),
			}
		}
		if toolDelta.Function.Arguments != "" {
			state.toolArgumentJSON[block] += toolDelta.Function.Arguments
		}
		events <- llm.Event{
			Type:         llm.EventToolCallDelta,
			ContentIndex: contentIndex,
			Delta:        toolDelta.Function.Arguments,
			ToolCall:     cloneToolCall(block),
			Partial:      cloneAssistantMessage(state.output),
		}
	}

	if choice.FinishReason != "" {
		state.finishReason = choice.FinishReason
	}
	return nil
}

// finish validates the provider finish reason, finalizes tool-call arguments,
// emits one end event per content block, and then emits the final done event.
func (state *streamState) finish(events chan<- llm.Event) error {
	stopReason, err := mapStopReason(state.finishReason)
	if err != nil {
		return err
	}
	state.output.StopReason = stopReason

	for contentIndex, rawContent := range state.output.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			events <- llm.Event{
				Type:         llm.EventTextEnd,
				ContentIndex: contentIndex,
				Content:      content.Text,
				Partial:      cloneAssistantMessage(state.output),
			}
		case *llm.ThinkingContent:
			events <- llm.Event{
				Type:         llm.EventThinkingEnd,
				ContentIndex: contentIndex,
				Content:      content.Thinking,
				Partial:      cloneAssistantMessage(state.output),
			}
		case *llm.ToolCall:
			content.Arguments = parseToolArguments(state.toolArgumentJSON[content])
			events <- llm.Event{
				Type:         llm.EventToolCallEnd,
				ContentIndex: contentIndex,
				ToolCall:     cloneToolCall(content),
				Partial:      cloneAssistantMessage(state.output),
			}
		}
	}

	events <- llm.Event{Type: llm.EventDone, Message: cloneAssistantMessage(state.output)}
	return nil
}

// ensureAssistantToolCall finds or creates the streaming tool call block for a
// delta, appending a new block to the message content on first sight and
// backfilling the id and name as they arrive across chunks. It tracks blocks by
// both stream index and id: some providers stream the index on every fragment,
// others only send the id once, so keying on a single field would merge distinct
// tool calls (or split one) depending on the provider.
func ensureAssistantToolCall(
	message *llm.AssistantMessage,
	byIndex map[int64]*llm.ToolCall,
	byID map[string]*llm.ToolCall,
	delta oai.ChatCompletionChunkChoiceDeltaToolCall,
) (*llm.ToolCall, int, bool) {
	hasIndex := delta.JSON.Index.Valid()

	var block *llm.ToolCall
	var ok bool
	if hasIndex {
		block, ok = byIndex[delta.Index]
	}
	if !ok && delta.ID != "" {
		block, ok = byID[delta.ID]
	}

	started := !ok
	if !ok {
		block = &llm.ToolCall{
			ID:        delta.ID,
			Name:      delta.Function.Name,
			Arguments: make(map[string]any),
		}
		message.Content = append(message.Content, block)
	}
	if hasIndex {
		byIndex[delta.Index] = block
	}
	if delta.ID != "" {
		byID[delta.ID] = block
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
	clone.Arguments = cloneJSONObject(toolCall.Arguments)
	return &clone
}

// parseToolArguments converts completed streamed JSON into the public argument
// object. OpenAI function arguments should be an object; malformed or incomplete
// JSON follows pi's tolerant behavior and becomes an empty object.
func parseToolArguments(raw string) map[string]any {
	if raw == "" {
		return make(map[string]any)
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil || arguments == nil {
		return make(map[string]any)
	}
	return arguments
}

func cloneJSONObject(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = cloneJSONValue(item)
	}
	return clone
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONObject(typed)
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneJSONValue(item)
		}
		return clone
	default:
		return value
	}
}

func emitError(events chan<- llm.Event, output llm.AssistantMessage, ctx context.Context, err error) {
	if ctx.Err() != nil {
		output.StopReason = llm.StopReasonAborted
		err = ctx.Err()
	} else {
		output.StopReason = llm.StopReasonError
	}
	output.ErrorMessage = err.Error()
	events <- llm.Event{Type: llm.EventError, Message: cloneAssistantMessage(output), Err: err}
}

func usageFrom(usage oai.CompletionUsage, model llm.Model) llm.Usage {
	cacheRead := usage.PromptTokensDetails.CachedTokens
	input := max(0, usage.PromptTokens-cacheRead)
	result := llm.Usage{
		Input:       input,
		Output:      usage.CompletionTokens,
		CacheRead:   cacheRead,
		TotalTokens: input + usage.CompletionTokens + cacheRead,
	}
	result.Cost = llm.CalculateCost(model, result)
	return result
}

// usageFromExtra reads usage from a non-standard extra field. Some providers
// (e.g. Moonshot) report token usage under choice.usage instead of the standard
// top-level chunk.usage. ok is false when the field is absent or null.
func usageFromExtra(fields map[string]respjson.Field, name string, model llm.Model) (llm.Usage, bool, error) {
	field, ok := fields[name]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return llm.Usage{}, false, nil
	}

	var usage oai.CompletionUsage
	if err := json.Unmarshal([]byte(field.Raw()), &usage); err != nil {
		return llm.Usage{}, false, fmt.Errorf("decode OpenAI %s field: %w", name, err)
	}
	return usageFrom(usage, model), true, nil
}

// reasoningFieldNames lists the delta fields that carry reasoning content across
// OpenAI-compatible providers, in precedence order. The first non-empty value
// wins so providers that echo the same content under several names (e.g. both
// reasoning_content and reasoning) do not duplicate it.
var reasoningFieldNames = []string{"reasoning_content", "reasoning", "reasoning_text"}

// extraReasoning returns the first non-empty reasoning delta among the known
// provider-specific field names.
func extraReasoning(fields map[string]respjson.Field) (string, error) {
	for _, name := range reasoningFieldNames {
		value, err := extraString(fields, name)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return "", nil
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

func mapStopReason(reason string) (llm.StopReason, error) {
	switch reason {
	case "stop", "end":
		return llm.StopReasonStop, nil
	case "length":
		return llm.StopReasonLength, nil
	case "tool_calls", "function_call":
		return llm.StopReasonToolUse, nil
	case "content_filter":
		return "", errors.New("OpenAI response was blocked by the content filter")
	case "":
		return "", errors.New("OpenAI stream ended without a finish reason")
	default:
		return "", fmt.Errorf("unsupported OpenAI finish reason %q", reason)
	}
}
