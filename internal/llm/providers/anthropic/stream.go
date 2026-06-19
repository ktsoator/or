package anthropic

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/internal/llm"
)

// streamState accumulates the partial assistant message while consuming one
// provider response. Anthropic identifies content blocks by a stream index;
// byIndex maps that index to the block being built, and toolJSON buffers each
// tool call's streamed argument JSON until the block closes.
type streamState struct {
	model    llm.Model
	output   llm.AssistantMessage
	byIndex  map[int64]llm.AssistantContent
	toolJSON map[int64]string
}

func newStreamState(model llm.Model) *streamState {
	output := llm.NewAssistantMessage(model)
	output.StopReason = llm.StopReasonStop
	return &streamState{
		model:    model,
		output:   output,
		byIndex:  make(map[int64]llm.AssistantContent),
		toolJSON: make(map[int64]string),
	}
}

// consumeStream owns the SDK stream and guarantees exactly one terminal event
// before closing events: done for a successful response or error for a failure.
func consumeStream(
	ctx context.Context,
	client sdk.Client,
	params sdk.MessageNewParams,
	model llm.Model,
	events chan<- llm.Event,
) {
	defer close(events)

	state := newStreamState(model)
	stream := client.Messages.NewStreaming(ctx, params)

	started := false
	for stream.Next() {
		if !started {
			started = true
			events <- llm.Event{Type: llm.EventStart, Partial: cloneAssistantMessage(state.output)}
		}
		if err := state.processEvent(stream.Current(), events); err != nil {
			emitError(events, state.output, ctx, err)
			return
		}
	}

	if err := stream.Err(); err != nil {
		emitError(events, state.output, ctx, err)
		return
	}
	if state.output.StopReason == llm.StopReasonError {
		message := cloneAssistantMessage(state.output)
		events <- llm.Event{Type: llm.EventError, Message: message, Err: errors.New(state.output.ErrorMessage)}
		return
	}

	events <- llm.Event{Type: llm.EventDone, Message: cloneAssistantMessage(state.output)}
}

func (state *streamState) processEvent(event sdk.MessageStreamEventUnion, events chan<- llm.Event) error {
	switch ev := event.AsAny().(type) {
	case sdk.MessageStartEvent:
		state.output.ResponseID = ev.Message.ID
		usage := ev.Message.Usage
		state.setUsage(usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
	case sdk.ContentBlockStartEvent:
		state.startBlock(ev, events)
	case sdk.ContentBlockDeltaEvent:
		state.deltaBlock(ev, events)
	case sdk.ContentBlockStopEvent:
		if err := state.stopBlock(ev, events); err != nil {
			return err
		}
	case sdk.MessageDeltaEvent:
		if ev.Delta.StopReason != "" {
			stopReason, errorMessage := mapStopReason(ev.Delta.StopReason)
			state.output.StopReason = stopReason
			if errorMessage != "" {
				state.output.ErrorMessage = errorMessage
			}
		}
		// Some proxies omit usage fields in message_delta; keep the message_start
		// value rather than clobbering it with a zero.
		usage := ev.Usage
		input, output := state.output.Usage.Input, state.output.Usage.Output
		cacheRead, cacheWrite := state.output.Usage.CacheRead, state.output.Usage.CacheWrite
		if usage.JSON.InputTokens.Valid() {
			input = usage.InputTokens
		}
		if usage.JSON.OutputTokens.Valid() {
			output = usage.OutputTokens
		}
		if usage.JSON.CacheReadInputTokens.Valid() {
			cacheRead = usage.CacheReadInputTokens
		}
		if usage.JSON.CacheCreationInputTokens.Valid() {
			cacheWrite = usage.CacheCreationInputTokens
		}
		state.setUsage(input, output, cacheRead, cacheWrite)
	}
	return nil
}

func (state *streamState) startBlock(ev sdk.ContentBlockStartEvent, events chan<- llm.Event) {
	var block llm.AssistantContent
	var eventType llm.EventType
	switch cb := ev.ContentBlock.AsAny().(type) {
	case sdk.TextBlock:
		block = &llm.TextContent{}
		eventType = llm.EventTextStart
	case sdk.ThinkingBlock:
		block = &llm.ThinkingContent{ThinkingSignature: cb.Signature}
		eventType = llm.EventThinkingStart
	case sdk.RedactedThinkingBlock:
		block = &llm.ThinkingContent{Thinking: "[Reasoning redacted]", ThinkingSignature: cb.Data, Redacted: true}
		eventType = llm.EventThinkingStart
	case sdk.ToolUseBlock:
		block = &llm.ToolCall{ID: cb.ID, Name: cb.Name, Arguments: map[string]any{}}
		state.toolJSON[ev.Index] = ""
		eventType = llm.EventToolCallStart
	default:
		// Server-side tool blocks are not modeled; ignore them.
		return
	}

	state.output.Content = append(state.output.Content, block)
	state.byIndex[ev.Index] = block
	contentIndex := len(state.output.Content) - 1

	event := llm.Event{Type: eventType, ContentIndex: contentIndex, Partial: cloneAssistantMessage(state.output)}
	if call, ok := block.(*llm.ToolCall); ok {
		event.ToolCall = cloneToolCall(call)
	}
	events <- event
}

func (state *streamState) deltaBlock(ev sdk.ContentBlockDeltaEvent, events chan<- llm.Event) {
	block, ok := state.byIndex[ev.Index]
	if !ok {
		return
	}
	contentIndex := assistantContentIndex(state.output.Content, block)

	switch delta := ev.Delta.AsAny().(type) {
	case sdk.TextDelta:
		if content, ok := block.(*llm.TextContent); ok {
			content.Text += delta.Text
			events <- llm.Event{
				Type:         llm.EventTextDelta,
				ContentIndex: contentIndex,
				Delta:        delta.Text,
				Partial:      cloneAssistantMessage(state.output),
			}
		}
	case sdk.ThinkingDelta:
		if content, ok := block.(*llm.ThinkingContent); ok {
			content.Thinking += delta.Thinking
			events <- llm.Event{
				Type:         llm.EventThinkingDelta,
				ContentIndex: contentIndex,
				Delta:        delta.Thinking,
				Partial:      cloneAssistantMessage(state.output),
			}
		}
	case sdk.InputJSONDelta:
		if content, ok := block.(*llm.ToolCall); ok {
			state.toolJSON[ev.Index] += delta.PartialJSON
			events <- llm.Event{
				Type:         llm.EventToolCallDelta,
				ContentIndex: contentIndex,
				Delta:        delta.PartialJSON,
				ToolCall:     cloneToolCall(content),
				Partial:      cloneAssistantMessage(state.output),
			}
		}
	case sdk.SignatureDelta:
		if content, ok := block.(*llm.ThinkingContent); ok {
			content.ThinkingSignature += delta.Signature
		}
	}
}

func (state *streamState) stopBlock(ev sdk.ContentBlockStopEvent, events chan<- llm.Event) error {
	block, ok := state.byIndex[ev.Index]
	if !ok {
		return nil
	}
	contentIndex := assistantContentIndex(state.output.Content, block)

	switch content := block.(type) {
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
		arguments, err := llm.ParseToolArguments(state.toolJSON[ev.Index])
		if err != nil {
			return fmt.Errorf("parse arguments for tool call %q (%s): %w", content.ID, content.Name, err)
		}
		content.Arguments = arguments
		events <- llm.Event{
			Type:         llm.EventToolCallEnd,
			ContentIndex: contentIndex,
			ToolCall:     cloneToolCall(content),
			Partial:      cloneAssistantMessage(state.output),
		}
	}
	return nil
}

func (state *streamState) setUsage(input, output, cacheRead, cacheWrite int64) {
	usage := llm.Usage{
		Input:       input,
		Output:      output,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		TotalTokens: input + output + cacheRead + cacheWrite,
	}
	usage.Cost = llm.CalculateCost(state.model, usage)
	state.output.Usage = usage
}

// mapStopReason maps an Anthropic stop reason to the package reason and, for
// failure cases, a message. Refusals and unknown reasons stop with an error.
func mapStopReason(reason sdk.StopReason) (llm.StopReason, string) {
	switch reason {
	case sdk.StopReasonEndTurn, sdk.StopReasonStopSequence, sdk.StopReasonPauseTurn:
		return llm.StopReasonStop, ""
	case sdk.StopReasonMaxTokens:
		return llm.StopReasonLength, ""
	case sdk.StopReasonToolUse:
		return llm.StopReasonToolUse, ""
	case sdk.StopReasonRefusal:
		return llm.StopReasonError, "the model refused to complete the request"
	default:
		return llm.StopReasonError, fmt.Sprintf("unhandled Anthropic stop reason %q", reason)
	}
}

func assistantContentIndex(content []llm.AssistantContent, target llm.AssistantContent) int {
	for i, candidate := range content {
		if candidate == target {
			return i
		}
	}
	return -1
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
