package anthropic

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/llm"
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
	// sawStop records whether the model signaled completion via a stop reason or
	// a message_stop event. A stream that closes cleanly without one was cut
	// short, so the accumulated (possibly truncated) response is not a success.
	sawStop bool
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
	writer := llm.NewStreamWriter(ctx, events, &state.output)
	// A panic in the SDK or event handling must surface as a terminal error event
	// rather than crashing the host process.
	defer func() {
		if r := recover(); r != nil {
			writer.Fail(fmt.Errorf("anthropic stream panicked: %v", r))
		}
	}()
	stream := client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	for stream.Next() {
		writer.Start()
		state.processEvent(stream.Current(), writer)
	}

	if err := stream.Err(); err != nil {
		writer.Fail(err)
		return
	}
	if state.output.StopReason == llm.StopReasonError {
		writer.Fail(errors.New(state.output.ErrorMessage))
		return
	}
	if !state.sawStop {
		writer.Fail(errors.New("anthropic stream ended without a stop reason"))
		return
	}

	writer.Done()
}

func (state *streamState) processEvent(event sdk.MessageStreamEventUnion, writer *llm.StreamWriter) {
	switch ev := event.AsAny().(type) {
	case sdk.MessageStartEvent:
		state.output.ResponseID = ev.Message.ID
		usage := ev.Message.Usage
		state.setUsage(usage.InputTokens, usage.OutputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens)
	case sdk.ContentBlockStartEvent:
		state.startBlock(ev, writer)
	case sdk.ContentBlockDeltaEvent:
		state.deltaBlock(ev, writer)
	case sdk.ContentBlockStopEvent:
		state.stopBlock(ev, writer)
	case sdk.MessageStopEvent:
		state.sawStop = true
	case sdk.MessageDeltaEvent:
		if ev.Delta.StopReason != "" {
			state.sawStop = true
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
}

func (state *streamState) startBlock(ev sdk.ContentBlockStartEvent, writer *llm.StreamWriter) {
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
		block = &llm.ToolCall{ID: cb.ID, Name: cb.Name, Arguments: llm.ParseToolArguments(string(cb.Input))}
		state.toolJSON[ev.Index] = ""
		eventType = llm.EventToolCallStart
	default:
		// Server-side tool blocks are not modeled; ignore them.
		return
	}

	state.output.Content = append(state.output.Content, block)
	state.byIndex[ev.Index] = block
	contentIndex := len(state.output.Content) - 1

	event := llm.Event{Type: eventType, ContentIndex: contentIndex}
	if call, ok := block.(*llm.ToolCall); ok {
		event.ToolCall = llm.CloneToolCall(call)
	}
	writer.Emit(event)
}

func (state *streamState) deltaBlock(ev sdk.ContentBlockDeltaEvent, writer *llm.StreamWriter) {
	block, ok := state.byIndex[ev.Index]
	if !ok {
		return
	}
	contentIndex := assistantContentIndex(state.output.Content, block)

	switch delta := ev.Delta.AsAny().(type) {
	case sdk.TextDelta:
		if content, ok := block.(*llm.TextContent); ok {
			content.Text += delta.Text
			writer.Emit(llm.Event{
				Type:         llm.EventTextDelta,
				ContentIndex: contentIndex,
				Delta:        delta.Text,
			})
		}
	case sdk.ThinkingDelta:
		if content, ok := block.(*llm.ThinkingContent); ok {
			content.Thinking += delta.Thinking
			writer.Emit(llm.Event{
				Type:         llm.EventThinkingDelta,
				ContentIndex: contentIndex,
				Delta:        delta.Thinking,
			})
		}
	case sdk.InputJSONDelta:
		if content, ok := block.(*llm.ToolCall); ok {
			state.toolJSON[ev.Index] += delta.PartialJSON
			writer.Emit(llm.Event{
				Type:         llm.EventToolCallDelta,
				ContentIndex: contentIndex,
				Delta:        delta.PartialJSON,
				ToolCall:     llm.CloneToolCall(content),
			})
		}
	case sdk.SignatureDelta:
		if content, ok := block.(*llm.ThinkingContent); ok {
			content.ThinkingSignature += delta.Signature
		}
	}
}

func (state *streamState) stopBlock(ev sdk.ContentBlockStopEvent, writer *llm.StreamWriter) {
	block, ok := state.byIndex[ev.Index]
	if !ok {
		return
	}
	contentIndex := assistantContentIndex(state.output.Content, block)

	switch content := block.(type) {
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
		// Reparse only when argument JSON streamed in via deltas. Some
		// Anthropic-compatible providers send the full input on content_block_start
		// with no deltas; overwriting with the empty buffer would drop it.
		if raw := state.toolJSON[ev.Index]; raw != "" {
			arguments, mode := llm.ParseToolArgumentsMode(raw)
			content.Arguments = arguments
			if diagnostic, ok := llm.ToolArgumentsDiagnostic(content.ID, content.Name, mode); ok {
				state.output.Diagnostics = append(state.output.Diagnostics, diagnostic)
			}
		}
		writer.Emit(llm.Event{
			Type:         llm.EventToolCallEnd,
			ContentIndex: contentIndex,
			ToolCall:     llm.CloneToolCall(content),
		})
	}
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
