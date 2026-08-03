package responses

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type responsesOutputItemState struct {
	id           string
	phase        string
	contentIndex int
	ended        bool
}

type responsesStreamState struct {
	model  llm.Model
	output llm.AssistantMessage

	items            map[int64]*responsesOutputItemState
	toolArgumentJSON map[int64]string
	refusal          string
	terminal         bool
}

func newResponsesStreamState(model llm.Model) *responsesStreamState {
	return &responsesStreamState{
		model:            model,
		output:           llm.NewAssistantMessage(model),
		items:            make(map[int64]*responsesOutputItemState),
		toolArgumentJSON: make(map[int64]string),
	}
}

func consumeResponsesStream(
	ctx context.Context,
	client oai.Client,
	params responses.ResponseNewParams,
	model llm.Model,
	events chan<- llm.Event,
) {
	defer close(events)

	state := newResponsesStreamState(model)
	writer := llm.NewStreamWriter(ctx, events, &state.output)
	defer func() {
		if recovered := recover(); recovered != nil {
			writer.Fail(fmt.Errorf("OpenAI Responses stream panicked: %v", recovered))
		}
	}()

	stream := client.Responses.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		writer.Start()
		if err := state.processEvent(stream.Current(), writer); err != nil {
			writer.Fail(err)
			return
		}
		if state.terminal {
			return
		}
	}
	if err := stream.Err(); err != nil {
		writer.Fail(err)
		return
	}
	writer.Fail(errors.New("OpenAI Responses stream ended without a terminal event"))
}

func (state *responsesStreamState) processEvent(event responses.ResponseStreamEventUnion, writer *llm.StreamWriter) error {
	switch event := event.AsAny().(type) {
	case responses.ResponseCreatedEvent:
		state.applyResponseMetadata(event.Response)
	case responses.ResponseInProgressEvent:
		state.applyResponseMetadata(event.Response)
	case responses.ResponseQueuedEvent:
		state.applyResponseMetadata(event.Response)
	case responses.ResponseOutputItemAddedEvent:
		return state.addOutputItem(event.OutputIndex, event.Item, writer)
	case responses.ResponseTextDeltaEvent:
		content, index, started := state.ensureText(event.OutputIndex, event.ItemID, "")
		if started {
			writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: index})
		}
		content.Text += event.Delta
		writer.Emit(llm.Event{Type: llm.EventTextDelta, ContentIndex: index, Delta: event.Delta})
	case responses.ResponseTextDoneEvent:
		state.setFinalText(event.OutputIndex, event.ItemID, event.Text, writer)
	case responses.ResponseReasoningSummaryTextDeltaEvent:
		state.appendThinkingDelta(event.OutputIndex, event.ItemID, event.Delta, writer)
	case responses.ResponseReasoningTextDeltaEvent:
		state.appendThinkingDelta(event.OutputIndex, event.ItemID, event.Delta, writer)
	case responses.ResponseReasoningSummaryTextDoneEvent:
		state.setFinalThinking(event.OutputIndex, event.ItemID, event.Text, writer)
	case responses.ResponseReasoningTextDoneEvent:
		state.setFinalThinking(event.OutputIndex, event.ItemID, event.Text, writer)
	case responses.ResponseFunctionCallArgumentsDeltaEvent:
		toolCall, index, started := state.ensureToolCall(event.OutputIndex, event.ItemID, "", "")
		if started {
			writer.Emit(llm.Event{Type: llm.EventToolCallStart, ContentIndex: index, ToolCall: llm.CloneToolCall(toolCall)})
		}
		state.toolArgumentJSON[event.OutputIndex] += event.Delta
		writer.Emit(llm.Event{
			Type:         llm.EventToolCallDelta,
			ContentIndex: index,
			Delta:        event.Delta,
			ToolCall:     llm.CloneToolCall(toolCall),
		})
	case responses.ResponseFunctionCallArgumentsDoneEvent:
		state.setFinalToolArguments(event.OutputIndex, event.ItemID, "", event.Name, event.Arguments, writer)
	case responses.ResponseOutputItemDoneEvent:
		if err := state.reconcileOutputItem(event.OutputIndex, event.Item, writer); err != nil {
			return err
		}
		return state.finalizeOutputItem(event.OutputIndex, writer)
	case responses.ResponseRefusalDeltaEvent:
		state.refusal += event.Delta
	case responses.ResponseRefusalDoneEvent:
		state.refusal = event.Refusal
	case responses.ResponseCompletedEvent:
		return state.complete(event.Response, writer)
	case responses.ResponseIncompleteEvent:
		return state.incomplete(event.Response, writer)
	case responses.ResponseFailedEvent:
		state.applyResponseMetadata(event.Response)
		state.terminal = true
		return responseFailure(event.Response)
	case responses.ResponseErrorEvent:
		state.terminal = true
		if event.Code != "" {
			return fmt.Errorf("OpenAI Responses error %s: %s", event.Code, event.Message)
		}
		return fmt.Errorf("OpenAI Responses error: %s", event.Message)
	}
	return nil
}

func (state *responsesStreamState) appendThinkingDelta(
	outputIndex int64,
	itemID, delta string,
	writer *llm.StreamWriter,
) {
	content, index, started := state.ensureThinking(outputIndex, itemID)
	if started {
		writer.Emit(llm.Event{Type: llm.EventThinkingStart, ContentIndex: index})
	}
	content.Thinking += delta
	writer.Emit(llm.Event{Type: llm.EventThinkingDelta, ContentIndex: index, Delta: delta})
}

func (state *responsesStreamState) addOutputItem(
	outputIndex int64,
	item responses.ResponseOutputItemUnion,
	writer *llm.StreamWriter,
) error {
	typeName := item.Type
	switch item := item.AsAny().(type) {
	case responses.ResponseOutputMessage:
		state.ensureItem(outputIndex, item.ID, string(item.Phase))
	case responses.ResponseReasoningItem:
		_, index, started := state.ensureThinking(outputIndex, item.ID)
		if started {
			writer.Emit(llm.Event{Type: llm.EventThinkingStart, ContentIndex: index})
		}
		state.reconcileReasoningItem(outputIndex, item, writer)
	case responses.ResponseFunctionToolCall:
		toolCall, index, started := state.ensureToolCall(outputIndex, item.ID, item.CallID, item.Name)
		if started {
			writer.Emit(llm.Event{Type: llm.EventToolCallStart, ContentIndex: index, ToolCall: llm.CloneToolCall(toolCall)})
		}
		state.setFinalToolArguments(outputIndex, item.ID, item.CallID, item.Name, item.Arguments, writer)
	default:
		return fmt.Errorf("unsupported OpenAI Responses output item type %q", typeName)
	}
	return nil
}

func (state *responsesStreamState) ensureItem(outputIndex int64, id, phase string) *responsesOutputItemState {
	if item, ok := state.items[outputIndex]; ok {
		if item.id == "" {
			item.id = id
		}
		if item.phase == "" {
			item.phase = phase
		}
		return item
	}
	item := &responsesOutputItemState{id: id, phase: phase, contentIndex: -1}
	state.items[outputIndex] = item
	return item
}

func (state *responsesStreamState) ensureText(outputIndex int64, itemID, phase string) (*llm.TextContent, int, bool) {
	item := state.ensureItem(outputIndex, itemID, phase)
	if item.contentIndex >= 0 {
		if content, ok := state.output.Content[item.contentIndex].(*llm.TextContent); ok && content != nil {
			return content, item.contentIndex, false
		}
	}
	content := &llm.TextContent{TextSignature: encodeResponsesSignature(responsesSignature{
		Kind: "text", ID: item.id, Phase: item.phase,
	})}
	state.output.Content = append(state.output.Content, content)
	item.contentIndex = len(state.output.Content) - 1
	return content, item.contentIndex, true
}

func (state *responsesStreamState) ensureThinking(outputIndex int64, itemID string) (*llm.ThinkingContent, int, bool) {
	item := state.ensureItem(outputIndex, itemID, "")
	if item.contentIndex >= 0 {
		if content, ok := state.output.Content[item.contentIndex].(*llm.ThinkingContent); ok && content != nil {
			return content, item.contentIndex, false
		}
	}
	content := &llm.ThinkingContent{ThinkingSignature: encodeResponsesSignature(responsesSignature{
		Kind: "reasoning", ID: item.id,
	})}
	state.output.Content = append(state.output.Content, content)
	item.contentIndex = len(state.output.Content) - 1
	return content, item.contentIndex, true
}

func (state *responsesStreamState) ensureToolCall(
	outputIndex int64,
	itemID, callID, name string,
) (*llm.ToolCall, int, bool) {
	item := state.ensureItem(outputIndex, itemID, "")
	if item.contentIndex >= 0 {
		if content, ok := state.output.Content[item.contentIndex].(*llm.ToolCall); ok && content != nil {
			if content.Name == "" {
				content.Name = name
			}
			if content.ID == "" || content.ID == "|" {
				content.ID = joinResponsesToolCallID(callID, item.id)
			}
			return content, item.contentIndex, false
		}
	}
	content := &llm.ToolCall{
		ID:        joinResponsesToolCallID(callID, item.id),
		Name:      name,
		Arguments: make(map[string]any),
	}
	state.output.Content = append(state.output.Content, content)
	item.contentIndex = len(state.output.Content) - 1
	return content, item.contentIndex, true
}

func joinResponsesToolCallID(callID, itemID string) string {
	if itemID == "" {
		return callID
	}
	return callID + "|" + itemID
}

func (state *responsesStreamState) setFinalText(
	outputIndex int64,
	itemID, text string,
	writer *llm.StreamWriter,
) {
	content, index, started := state.ensureText(outputIndex, itemID, "")
	if started {
		writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: index})
	}
	delta := missingFinalDelta(content.Text, text)
	content.Text = preferFinalValue(content.Text, text)
	if delta != "" {
		writer.Emit(llm.Event{Type: llm.EventTextDelta, ContentIndex: index, Delta: delta})
	}
}

func (state *responsesStreamState) setFinalThinking(
	outputIndex int64,
	itemID, text string,
	writer *llm.StreamWriter,
) {
	content, index, started := state.ensureThinking(outputIndex, itemID)
	if started {
		writer.Emit(llm.Event{Type: llm.EventThinkingStart, ContentIndex: index})
	}
	delta := missingFinalDelta(content.Thinking, text)
	content.Thinking = preferFinalValue(content.Thinking, text)
	if delta != "" {
		writer.Emit(llm.Event{Type: llm.EventThinkingDelta, ContentIndex: index, Delta: delta})
	}
}

func (state *responsesStreamState) setFinalToolArguments(
	outputIndex int64,
	itemID, callID, name, arguments string,
	writer *llm.StreamWriter,
) {
	toolCall, index, started := state.ensureToolCall(outputIndex, itemID, callID, name)
	if started {
		writer.Emit(llm.Event{Type: llm.EventToolCallStart, ContentIndex: index, ToolCall: llm.CloneToolCall(toolCall)})
	}
	delta := missingFinalDelta(state.toolArgumentJSON[outputIndex], arguments)
	state.toolArgumentJSON[outputIndex] = preferFinalValue(state.toolArgumentJSON[outputIndex], arguments)
	if delta != "" {
		writer.Emit(llm.Event{
			Type:         llm.EventToolCallDelta,
			ContentIndex: index,
			Delta:        delta,
			ToolCall:     llm.CloneToolCall(toolCall),
		})
	}
}

func preferFinalValue(streamed, final string) string {
	if final == "" {
		return streamed
	}
	return final
}

func missingFinalDelta(streamed, final string) string {
	if final == "" || final == streamed {
		return ""
	}
	if strings.HasPrefix(final, streamed) {
		return strings.TrimPrefix(final, streamed)
	}
	return ""
}

func (state *responsesStreamState) reconcileOutputItem(
	outputIndex int64,
	item responses.ResponseOutputItemUnion,
	writer *llm.StreamWriter,
) error {
	typeName := item.Type
	switch item := item.AsAny().(type) {
	case responses.ResponseOutputMessage:
		state.reconcileOutputMessage(outputIndex, item, writer)
	case responses.ResponseReasoningItem:
		state.reconcileReasoningItem(outputIndex, item, writer)
	case responses.ResponseFunctionToolCall:
		state.reconcileFunctionToolCall(outputIndex, item, writer)
	default:
		return fmt.Errorf("unsupported OpenAI Responses output item type %q", typeName)
	}
	return nil
}

func (state *responsesStreamState) reconcileOutputMessage(
	outputIndex int64,
	item responses.ResponseOutputMessage,
	writer *llm.StreamWriter,
) {
	state.ensureItem(outputIndex, item.ID, string(item.Phase))
	var text strings.Builder
	for _, part := range item.Content {
		switch part := part.AsAny().(type) {
		case responses.ResponseOutputText:
			text.WriteString(part.Text)
		case responses.ResponseOutputRefusal:
			state.refusal += part.Refusal
		}
	}
	if text.Len() > 0 {
		state.setFinalText(outputIndex, item.ID, text.String(), writer)
		content, _, _ := state.ensureText(outputIndex, item.ID, string(item.Phase))
		content.TextSignature = encodeResponsesSignature(responsesSignature{
			Kind: "text", ID: item.ID, Phase: string(item.Phase),
		})
	}
}

func (state *responsesStreamState) reconcileReasoningItem(
	outputIndex int64,
	item responses.ResponseReasoningItem,
	writer *llm.StreamWriter,
) {
	content, index, started := state.ensureThinking(outputIndex, item.ID)
	if started {
		writer.Emit(llm.Event{Type: llm.EventThinkingStart, ContentIndex: index})
	}
	var thinking strings.Builder
	for partIndex, summary := range item.Summary {
		if partIndex > 0 {
			thinking.WriteString("\n\n")
		}
		thinking.WriteString(summary.Text)
	}
	if thinking.Len() == 0 {
		for partIndex, reasoning := range item.Content {
			if partIndex > 0 {
				thinking.WriteString("\n\n")
			}
			thinking.WriteString(reasoning.Text)
		}
	}
	if thinking.Len() > 0 {
		state.setFinalThinking(outputIndex, item.ID, thinking.String(), writer)
		content, _, _ = state.ensureThinking(outputIndex, item.ID)
		content.Redacted = false
	} else if item.EncryptedContent != "" && content.Thinking == "" && item.Status != responses.ResponseReasoningItemStatusInProgress {
		content.Thinking = "[Reasoning redacted]"
		content.Redacted = true
	}
	content.ThinkingSignature = encodeResponsesSignature(responsesSignature{
		Kind: "reasoning", ID: item.ID, EncryptedContent: item.EncryptedContent,
	})
}

func (state *responsesStreamState) reconcileFunctionToolCall(
	outputIndex int64,
	item responses.ResponseFunctionToolCall,
	writer *llm.StreamWriter,
) {
	toolCall, index, started := state.ensureToolCall(outputIndex, item.ID, item.CallID, item.Name)
	if started {
		writer.Emit(llm.Event{Type: llm.EventToolCallStart, ContentIndex: index, ToolCall: llm.CloneToolCall(toolCall)})
	}
	toolCall.ID = joinResponsesToolCallID(item.CallID, item.ID)
	toolCall.Name = item.Name
	state.setFinalToolArguments(outputIndex, item.ID, item.CallID, item.Name, item.Arguments, writer)
}

func (state *responsesStreamState) finalizeOutputItem(outputIndex int64, writer *llm.StreamWriter) error {
	item, ok := state.items[outputIndex]
	if !ok || item.ended || item.contentIndex < 0 {
		return nil
	}
	item.ended = true
	switch content := state.output.Content[item.contentIndex].(type) {
	case *llm.TextContent:
		writer.Emit(llm.Event{Type: llm.EventTextEnd, ContentIndex: item.contentIndex, Content: content.Text})
	case *llm.ThinkingContent:
		writer.Emit(llm.Event{Type: llm.EventThinkingEnd, ContentIndex: item.contentIndex, Content: content.Thinking})
	case *llm.ToolCall:
		arguments, mode := llm.ParseToolArgumentsMode(state.toolArgumentJSON[outputIndex])
		content.Arguments = arguments
		if diagnostic, ok := llm.ToolArgumentsDiagnostic(content.ID, content.Name, mode); ok {
			state.output.Diagnostics = append(state.output.Diagnostics, diagnostic)
		}
		writer.Emit(llm.Event{Type: llm.EventToolCallEnd, ContentIndex: item.contentIndex, ToolCall: llm.CloneToolCall(content)})
	}
	return nil
}

func (state *responsesStreamState) reconcileResponse(response responses.Response, writer *llm.StreamWriter) error {
	state.applyResponseMetadata(response)
	finalized := make(map[int64]bool, len(response.Output))
	for outputIndex, item := range response.Output {
		index := int64(outputIndex)
		if err := state.reconcileOutputItem(index, item, writer); err != nil {
			return err
		}
		if err := state.finalizeOutputItem(index, writer); err != nil {
			return err
		}
		finalized[index] = true
	}
	remaining := make([]int64, 0, len(state.items)-len(finalized))
	for outputIndex := range state.items {
		if !finalized[outputIndex] {
			remaining = append(remaining, outputIndex)
		}
	}
	slices.Sort(remaining)
	for _, outputIndex := range remaining {
		if err := state.finalizeOutputItem(outputIndex, writer); err != nil {
			return err
		}
	}
	return nil
}

func (state *responsesStreamState) complete(response responses.Response, writer *llm.StreamWriter) error {
	if err := state.reconcileResponse(response, writer); err != nil {
		return err
	}
	state.terminal = true
	if state.refusal != "" {
		return fmt.Errorf("OpenAI Responses refusal: %s", state.refusal)
	}
	state.output.StopReason = llm.StopReasonStop
	for _, content := range state.output.Content {
		if _, ok := content.(*llm.ToolCall); ok {
			state.output.StopReason = llm.StopReasonToolUse
			break
		}
	}
	writer.Done()
	return nil
}

func (state *responsesStreamState) incomplete(response responses.Response, writer *llm.StreamWriter) error {
	if err := state.reconcileResponse(response, writer); err != nil {
		return err
	}
	state.terminal = true
	if response.IncompleteDetails.Reason == "max_output_tokens" {
		state.output.StopReason = llm.StopReasonLength
		writer.Done()
		return nil
	}
	if response.IncompleteDetails.Reason == "content_filter" {
		return errors.New("OpenAI Responses output was blocked by the content filter")
	}
	return fmt.Errorf("OpenAI Responses incomplete: %s", response.IncompleteDetails.Reason)
}

func (state *responsesStreamState) applyResponseMetadata(response responses.Response) {
	if response.ID != "" {
		state.output.ResponseID = response.ID
	}
	if response.Model != "" && string(response.Model) != state.model.ID {
		state.output.ResponseModel = string(response.Model)
	}
	if response.JSON.Usage.Valid() {
		state.output.Usage = responsesUsageFrom(response.Usage, state.model)
	}
}

func responsesUsageFrom(usage responses.ResponseUsage, model llm.Model) llm.Usage {
	cacheRead := usage.InputTokensDetails.CachedTokens
	cacheWrite := usage.InputTokensDetails.CacheWriteTokens
	inputUnknown := usage.InputTokens < cacheRead+cacheWrite
	input := int64(0)
	if !inputUnknown {
		input = usage.InputTokens - cacheRead - cacheWrite
	}
	result := llm.Usage{
		Input:        input,
		InputUnknown: inputUnknown,
		Output:       usage.OutputTokens,
		CacheRead:    cacheRead,
		CacheWrite:   cacheWrite,
		TotalTokens:  usage.TotalTokens,
	}
	result.Cost = llm.CalculateCost(model, result)
	return result
}

func responseFailure(response responses.Response) error {
	if response.Error.Message == "" {
		return errors.New("OpenAI Responses request failed")
	}
	if response.Error.Code != "" {
		return fmt.Errorf("OpenAI Responses request failed (%s): %s", response.Error.Code, response.Error.Message)
	}
	return fmt.Errorf("OpenAI Responses request failed: %s", response.Error.Message)
}
