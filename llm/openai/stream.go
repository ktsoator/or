package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ktsoator/or/llm"
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
	writer := llm.NewStreamWriter(ctx, events, &state.output)
	// A panic in the SDK or chunk handling must surface as a terminal error event
	// rather than crashing the host process.
	defer func() {
		if r := recover(); r != nil {
			writer.Fail(fmt.Errorf("openai stream panicked: %v", r))
		}
	}()
	stream := client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	for stream.Next() {
		writer.Start()
		if err := state.processChunk(stream.Current(), writer); err != nil {
			writer.Fail(err)
			return
		}
	}

	if err := stream.Err(); err != nil {
		writer.Fail(err)
		return
	}
	if err := state.finish(writer); err != nil {
		writer.Fail(err)
	}
}

// processChunk merges one provider chunk into the partial assistant message and
// emits the corresponding delta events.
func (state *streamState) processChunk(chunk oai.ChatCompletionChunk, writer *llm.StreamWriter) error {
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

	reasoningDelta, reasoningField, err := extraReasoning(choice.Delta.JSON.ExtraFields)
	if err != nil {
		return err
	}
	if reasoningDelta != "" {
		signature := reasoningSignature(state.model, reasoningField)
		content, contentIndex, started := ensureAssistantThinking(&state.output, signature)
		if started {
			writer.Emit(llm.Event{
				Type:         llm.EventThinkingStart,
				ContentIndex: contentIndex,
			})
		}
		content.Thinking += reasoningDelta
		writer.Emit(llm.Event{
			Type:         llm.EventThinkingDelta,
			ContentIndex: contentIndex,
			Delta:        reasoningDelta,
		})
	}

	if choice.Delta.Content != "" {
		content, contentIndex, started := ensureAssistantText(&state.output)
		if started {
			writer.Emit(llm.Event{
				Type:         llm.EventTextStart,
				ContentIndex: contentIndex,
			})
		}
		content.Text += choice.Delta.Content
		writer.Emit(llm.Event{
			Type:         llm.EventTextDelta,
			ContentIndex: contentIndex,
			Delta:        choice.Delta.Content,
		})
	}

	for _, toolDelta := range choice.Delta.ToolCalls {
		block, contentIndex, started := ensureAssistantToolCall(
			&state.output,
			state.toolCallsByIndex,
			state.toolCallsByID,
			toolDelta,
		)
		if started {
			writer.Emit(llm.Event{
				Type:         llm.EventToolCallStart,
				ContentIndex: contentIndex,
				ToolCall:     llm.CloneToolCall(block),
			})
		}
		if toolDelta.Function.Arguments != "" {
			state.toolArgumentJSON[block] += toolDelta.Function.Arguments
		}
		writer.Emit(llm.Event{
			Type:         llm.EventToolCallDelta,
			ContentIndex: contentIndex,
			Delta:        toolDelta.Function.Arguments,
			ToolCall:     llm.CloneToolCall(block),
		})
	}

	if err := applyReasoningDetails(choice.Delta.JSON.ExtraFields, state.toolCallsByID); err != nil {
		return err
	}

	if choice.FinishReason != "" {
		state.finishReason = choice.FinishReason
	}
	return nil
}

// finish validates the provider finish reason, finalizes tool-call arguments,
// emits one end event per content block, and then emits the final done event.
func (state *streamState) finish(writer *llm.StreamWriter) error {
	stopReason, err := mapStopReason(state.finishReason)
	if err != nil {
		return err
	}
	state.output.StopReason = stopReason

	for contentIndex, rawContent := range state.output.Content {
		switch content := rawContent.(type) {
		case *llm.TextContent:
			writer.Emit(llm.Event{
				Type:         llm.EventTextEnd,
				ContentIndex: contentIndex,
				Content:      content.Text,
			})
		case *llm.ThinkingContent:
			writer.Emit(llm.Event{
				Type:         llm.EventThinkingEnd,
				ContentIndex: contentIndex,
				Content:      content.Thinking,
			})
		case *llm.ToolCall:
			arguments, mode := llm.ParseToolArgumentsMode(state.toolArgumentJSON[content])
			content.Arguments = arguments
			if diagnostic, ok := llm.ToolArgumentsDiagnostic(content.ID, content.Name, mode); ok {
				state.output.Diagnostics = append(state.output.Diagnostics, diagnostic)
			}
			writer.Emit(llm.Event{
				Type:         llm.EventToolCallEnd,
				ContentIndex: contentIndex,
				ToolCall:     llm.CloneToolCall(content),
			})
		}
	}

	writer.Done()
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

// ensureAssistantThinking returns the message's reasoning block, creating it on
// first sight. signature records which provider field carried the reasoning so
// it can be replayed under the same field on later turns.
func ensureAssistantThinking(message *llm.AssistantMessage, signature string) (*llm.ThinkingContent, int, bool) {
	for i, rawContent := range message.Content {
		if content, ok := rawContent.(*llm.ThinkingContent); ok && content != nil {
			return content, i, false
		}
	}
	content := &llm.ThinkingContent{ThinkingSignature: signature}
	message.Content = append(message.Content, content)
	return content, len(message.Content) - 1, true
}

// reasoningSignature records the source field for a reasoning delta. opencode-go
// streams reasoning under "reasoning" but replays it as "reasoning_content", so
// it is normalized here to match the field accepted on the next turn.
func reasoningSignature(model llm.Model, field string) string {
	if model.Provider == "opencode-go" && field == "reasoning" {
		return "reasoning_content"
	}
	return field
}

func assistantContentIndex(content []llm.AssistantContent, target llm.AssistantContent) int {
	for i, candidate := range content {
		if candidate == target {
			return i
		}
	}
	return -1
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
// provider-specific field names, along with the field it came from so the
// reasoning can be replayed under the same field.
func extraReasoning(fields map[string]respjson.Field) (string, string, error) {
	for _, name := range reasoningFieldNames {
		value, err := extraString(fields, name)
		if err != nil {
			return "", "", err
		}
		if value != "" {
			return value, name, nil
		}
	}
	return "", "", nil
}

// applyReasoningDetails binds OpenRouter-style encrypted reasoning to the tool
// call it belongs to. The delta's reasoning_details array carries entries keyed
// by tool-call id; each "reasoning.encrypted" entry's raw JSON is stored on the
// matching tool call's thought signature so it can be replayed verbatim on the
// next turn. Entries without a matching call, or of other types, are ignored.
func applyReasoningDetails(fields map[string]respjson.Field, byID map[string]*llm.ToolCall) error {
	field, ok := fields["reasoning_details"]
	if !ok || field.Raw() == "" || field.Raw() == "null" {
		return nil
	}
	var details []json.RawMessage
	if err := json.Unmarshal([]byte(field.Raw()), &details); err != nil {
		return fmt.Errorf("decode OpenAI reasoning_details field: %w", err)
	}
	for _, raw := range details {
		var entry struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return fmt.Errorf("decode OpenAI reasoning_details entry: %w", err)
		}
		if entry.Type != "reasoning.encrypted" || entry.ID == "" || entry.Data == "" {
			continue
		}
		if call, ok := byID[entry.ID]; ok && call != nil {
			call.ThoughtSignature = string(raw)
		}
	}
	return nil
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
