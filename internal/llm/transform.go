package llm

import (
	"slices"
	"strings"
)

const (
	// These placeholders preserve the fact that visual information existed in
	// the conversation while keeping the request valid for text-only models.
	// Separate text identifies whether the unavailable image came from the user
	// or from a tool result.
	nonVisionUserImagePlaceholder = "(image omitted: model does not support images)"
	nonVisionToolImagePlaceholder = "(tool image omitted: model does not support images)"

	// Tool protocols require every assistant tool call to have a result. This
	// text marks a result synthesized to repair an incomplete transcript; it is
	// not a real response produced by the tool.
	orphanedToolResultText = "No result provided"
)

// TransformMessages prepares the library's provider-independent conversation
// history for replay against model. Provider adapters should call it before
// translating Message values into their own wire-format message types.
//
// Transformation happens per request instead of modifying the Agent's canonical
// history. The same history may later be sent to a model with different image,
// reasoning, or tool capabilities. Mutating stored history would make that
// model switch lose information permanently.
//
// Shared transformations are applied in this order:
//  1. Replace images with descriptive text when the target model is text-only.
//  2. Reconcile assistant turns produced by a different model: keep reasoning for
//     the same model, downgrade it to text otherwise, and normalize tool-call
//     identifiers via normalizeToolCallID when crossing providers.
//  3. Drop assistant turns terminated by an error or cancellation because they
//     may contain partial reasoning or half-streamed tool calls.
//  4. Insert synthetic error results for tool calls with no matching result
//     before the conversation continues or ends.
//
// normalizeToolCallID rewrites a tool-call ID for the target provider; pass nil
// to leave identifiers unchanged.
//
// The returned slice is new and this function does not mutate messages. Message
// objects requiring no changes may still be shared with the input, so callers
// should treat the input and result as immutable.
func TransformMessages(messages []Message, model Model, normalizeToolCallID func(string) string) []Message {
	transformed := downgradeUnsupportedImages(messages, model)
	transformed = reconcileAssistantHistory(transformed, model, normalizeToolCallID)
	return synthesizeOrphanedToolResults(transformed)
}

// reconcileAssistantHistory rewrites each assistant turn for the target model and
// remaps the tool results that answer any renamed tool calls. Tool calls precede
// their results in a transcript, so a single forward pass can record an ID change
// on the assistant turn and apply it to later results.
func reconcileAssistantHistory(messages []Message, model Model, normalizeToolCallID func(string) string) []Message {
	idRemap := make(map[string]string)
	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		switch typed := message.(type) {
		case *AssistantMessage:
			if typed == nil {
				result = append(result, message)
				continue
			}
			result = append(result, reconcileAssistantMessage(typed, model, normalizeToolCallID, idRemap))
		case *ToolResultMessage:
			if typed != nil {
				if newID, ok := idRemap[typed.ToolCallID]; ok && newID != typed.ToolCallID {
					clone := *typed
					clone.ToolCallID = newID
					result = append(result, &clone)
					continue
				}
			}
			result = append(result, message)
		default:
			result = append(result, message)
		}
	}
	return result
}

// reconcileAssistantMessage rewrites one assistant turn's content for the target
// model. sameModel is true only when provider, protocol, and model id all match,
// which is the condition for replaying model-specific reasoning and signatures.
func reconcileAssistantMessage(
	message *AssistantMessage,
	model Model,
	normalizeToolCallID func(string) string,
	idRemap map[string]string,
) Message {
	sameModel := message.Provider == model.Provider &&
		message.Protocol == model.Protocol &&
		message.Model == model.ID

	content := make([]AssistantContent, 0, len(message.Content))
	for _, block := range message.Content {
		switch typed := block.(type) {
		case *ThinkingContent:
			if kept := reconcileThinking(typed, sameModel); kept != nil {
				content = append(content, kept)
			}
		case *TextContent:
			if sameModel || typed == nil {
				content = append(content, block)
			} else {
				// A text signature is only valid for its originating model.
				content = append(content, &TextContent{Text: typed.Text})
			}
		case *ToolCall:
			content = append(content, reconcileToolCall(typed, sameModel, normalizeToolCallID, idRemap))
		default:
			content = append(content, block)
		}
	}

	clone := *message
	clone.Content = content
	return &clone
}

// reconcileThinking decides how a stored reasoning block is replayed. Redacted
// reasoning is an opaque encrypted payload only the original model can consume.
// A signed block is kept for the same model even when empty. Otherwise empty
// reasoning is dropped, same-model reasoning is kept, and reasoning from another
// model is downgraded to plain text so it survives without a valid signature.
func reconcileThinking(content *ThinkingContent, sameModel bool) AssistantContent {
	if content == nil {
		return nil
	}
	if content.Redacted {
		if sameModel {
			return content
		}
		return nil
	}
	if sameModel && content.ThinkingSignature != "" {
		return content
	}
	if strings.TrimSpace(content.Thinking) == "" {
		return nil
	}
	if sameModel {
		return content
	}
	return &TextContent{Text: content.Thinking}
}

// reconcileToolCall keeps a same-model tool call intact. Crossing models it drops
// the provider-specific thought signature and, when a normalizer is supplied,
// rewrites the call ID, recording the change so the matching result is remapped.
func reconcileToolCall(
	call *ToolCall,
	sameModel bool,
	normalizeToolCallID func(string) string,
	idRemap map[string]string,
) AssistantContent {
	if call == nil || sameModel {
		return call
	}
	clone := *call
	clone.ThoughtSignature = ""
	if normalizeToolCallID != nil {
		if newID := normalizeToolCallID(call.ID); newID != call.ID {
			idRemap[call.ID] = newID
			clone.ID = newID
		}
	}
	return &clone
}

// downgradeUnsupportedImages projects image-bearing user and tool-result
// messages into a representation accepted by a text-only model. Models that
// support images receive a new outer slice with their messages unchanged.
func downgradeUnsupportedImages(messages []Message, model Model) []Message {
	if slices.Contains(model.Input, ModelInputImage) {
		return append([]Message(nil), messages...)
	}

	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		switch typed := message.(type) {
		case *UserMessage:
			if typed == nil {
				result = append(result, message)
				continue
			}
			result = append(result, &UserMessage{
				Content: downgradeUserImages(typed.Content, nonVisionUserImagePlaceholder),
			})
		case *ToolResultMessage:
			if typed == nil {
				result = append(result, message)
				continue
			}
			clone := *typed
			clone.Content = downgradeToolImages(typed.Content, nonVisionToolImagePlaceholder)
			result = append(result, &clone)
		default:
			result = append(result, message)
		}
	}
	return result
}

// downgradeUserImages replaces each consecutive run of user image blocks with
// one placeholder. Collapsing runs avoids filling the prompt with repeated
// identical text when a message contains several adjacent images.
func downgradeUserImages(content []UserContent, placeholder string) []UserContent {
	result := make([]UserContent, 0, len(content))
	previousWasPlaceholder := false
	for _, block := range content {
		if _, ok := block.(*ImageContent); ok {
			if !previousWasPlaceholder {
				result = append(result, &TextContent{Text: placeholder})
			}
			previousWasPlaceholder = true
			continue
		}
		result = append(result, block)
		text, ok := block.(*TextContent)
		previousWasPlaceholder = ok && text != nil && text.Text == placeholder
	}
	return result
}

// downgradeToolImages applies the same rule to tool-result content. It remains
// separate because user and tool-result blocks use different Go interfaces.
func downgradeToolImages(content []ToolResultContent, placeholder string) []ToolResultContent {
	result := make([]ToolResultContent, 0, len(content))
	previousWasPlaceholder := false
	for _, block := range content {
		if _, ok := block.(*ImageContent); ok {
			if !previousWasPlaceholder {
				result = append(result, &TextContent{Text: placeholder})
			}
			previousWasPlaceholder = true
			continue
		}
		result = append(result, block)
		text, ok := block.(*TextContent)
		previousWasPlaceholder = ok && text != nil && text.Text == placeholder
	}
	return result
}

// synthesizeOrphanedToolResults repairs the tool protocol invariant that an
// assistant tool-call batch must receive one ToolResultMessage per call before
// another assistant or user turn begins.
//
// pending holds the latest assistant tool-call batch. answered records the call
// IDs already covered by results. flush inserts error results for the remaining
// calls whenever a conversation boundary closes the batch.
func synthesizeOrphanedToolResults(messages []Message) []Message {
	result := make([]Message, 0, len(messages))

	var pending []*ToolCall
	answered := make(map[string]bool)
	flush := func() {
		for _, call := range pending {
			if call != nil && !answered[call.ID] {
				result = append(result, &ToolResultMessage{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Content:    []ToolResultContent{&TextContent{Text: orphanedToolResultText}},
					IsError:    true,
				})
			}
		}
		pending = nil
		answered = make(map[string]bool)
	}

	for _, message := range messages {
		switch typed := message.(type) {
		case *AssistantMessage:
			// A new assistant turn closes the preceding tool-call batch.
			flush()
			if typed == nil {
				result = append(result, message)
				continue
			}
			if typed.StopReason == StopReasonError || typed.StopReason == StopReasonAborted {
				// Failed streams may end in partial reasoning or incomplete tool
				// arguments, so replaying them is unsafe.
				continue
			}
			if calls := assistantToolCalls(typed); len(calls) > 0 {
				pending = calls
				answered = make(map[string]bool)
			}
			result = append(result, message)
		case *ToolResultMessage:
			// Results may arrive in any order within the current batch.
			if typed != nil {
				answered[typed.ToolCallID] = true
			}
			result = append(result, message)
		case *UserMessage:
			// A user turn cannot legally skip unresolved tool calls.
			flush()
			result = append(result, message)
		default:
			result = append(result, message)
		}
	}

	// Repair unresolved calls when the transcript ends with a tool-call batch.
	flush()
	return result
}

// assistantToolCalls extracts tool-call blocks and ignores text, thinking, and
// typed-nil tool-call values.
func assistantToolCalls(message *AssistantMessage) []*ToolCall {
	var calls []*ToolCall
	for _, content := range message.Content {
		if call, ok := content.(*ToolCall); ok && call != nil {
			calls = append(calls, call)
		}
	}
	return calls
}
