package engine

import (
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// EventType identifies a UI-neutral coding-session event. Product adapters
// render these events for their own transport instead of depending on the
// lower-level agent event model.
type EventType string

const (
	RunStarted           EventType = "run_started"
	UserMessageCompleted EventType = "user_message_completed"
	TextDelta            EventType = "text_delta"
	ThinkingDelta        EventType = "thinking_delta"
	ToolStarted          EventType = "tool_started"
	ToolFinished         EventType = "tool_finished"
	MessageCompleted     EventType = "message_completed"
	TurnDiscarded        EventType = "turn_discarded"
	CompactionStarted    EventType = "compaction_started"
	CompactionCompleted  EventType = "compaction_completed"
	CompactionFailed     EventType = "compaction_failed"
	RunCompleted         EventType = "run_completed"
)

// Event is the stable event contract exposed by Session. Fields are populated
// according to Type; presentation-specific concerns such as ANSI styling, JSON
// field names, SSE framing, and Markdown rendering stay in product adapters.
type Event struct {
	Type EventType

	// Streaming and completed assistant content.
	Delta  string
	Text   string
	Images []llm.ImageContent
	// FinalResponse distinguishes a user-visible completed reply from an
	// assistant message that paused only to call tools.
	FinalResponse bool

	// Tool lifecycle data.
	ToolCallID string
	ToolName   string
	ToolArgs   any
	ToolResult string
	// ToolDetails is the tool's structured result (for example a tools.FileChange
	// or tools.MutationFailure), when it produced one. Product shells render it;
	// ToolResult remains the text fallback. It is not persisted, so it is present
	// on live events but absent when history is replayed.
	ToolDetails any
	IsError     bool

	// Usage is one assistant request's consumption on MessageCompleted and the
	// aggregate consumption on RunCompleted. Product adapters may accumulate
	// tool-use requests until FinalResponse to show one total per visible reply.
	Usage llm.Usage

	// Response metadata identifies the exact provider request represented by a
	// MessageCompleted event. It lets product shells build durable, per-model
	// usage reports without inferring the active model from mutable UI state.
	Provider      string
	Model         string
	ResponseModel string
	ResponseID    string
	Timestamp     time.Time
	// Automatic distinguishes context maintenance performed inside an active run
	// from an explicit Compact call. Error is populated on CompactionFailed.
	Automatic bool
	Error     string

	// Run timing is populated on RunStarted and RunCompleted. It measures the
	// full invocation, including model calls, tools, approvals, retries, and any
	// steering or follow-up work consumed before the run ends.
	StartedAt   time.Time
	CompletedAt time.Time
}

// projectAgentEvent maps a low-level agent event into the stable coding event
// contract. Agent events without product-visible meaning are omitted.
func projectAgentEvent(ev agent.AgentEvent) (Event, bool) {
	switch ev.Type {
	case agent.AgentStart:
		// Session.run emits one outer RunStarted event. AgentStart can occur again
		// during an application-level retry and must not reset the visible timer.
		return Event{}, false

	case agent.MessageUpdate:
		if ev.LLMEvent == nil {
			return Event{}, false
		}
		switch ev.LLMEvent.Type {
		case llm.EventTextDelta:
			return Event{Type: TextDelta, Delta: ev.LLMEvent.Delta}, true
		case llm.EventThinkingDelta:
			return Event{Type: ThinkingDelta, Delta: ev.LLMEvent.Delta}, true
		default:
			return Event{}, false
		}

	case agent.ToolStart:
		return Event{
			Type:       ToolStarted,
			ToolCallID: ev.ToolCallID,
			ToolName:   ev.ToolName,
			ToolArgs:   ev.Args,
		}, true

	case agent.ToolEnd:
		return Event{
			Type:        ToolFinished,
			ToolCallID:  ev.ToolCallID,
			ToolName:    ev.ToolName,
			ToolResult:  eventToolResultText(ev.Result),
			ToolDetails: eventToolResultDetails(ev.Result),
			IsError:     ev.IsError,
		}, true

	case agent.MessageEnd:
		if text, images, ok := eventUserMessage(ev.Message); ok {
			return Event{Type: UserMessageCompleted, Text: text, Images: images}, true
		}
		assistant, ok := eventAssistantMessage(ev.Message)
		if !ok {
			return Event{}, false
		}
		return Event{
			Type: MessageCompleted,
			Text: displayAssistantText(assistant),
			FinalResponse: assistant.StopReason != llm.StopReasonToolUse &&
				assistant.StopReason != llm.StopReasonError &&
				assistant.StopReason != llm.StopReasonAborted,
			Usage:         assistant.Usage,
			Provider:      assistant.Provider,
			Model:         assistant.Model,
			ResponseModel: assistant.ResponseModel,
			ResponseID:    assistant.ResponseID,
			Timestamp:     time.UnixMilli(assistant.Timestamp).UTC(),
			CompletedAt:   time.Now().UTC(),
		}, true

	case agent.AgentEnd:
		// Session.run emits RunCompleted after retries and persistence have finished.
		return Event{}, false

	default:
		return Event{}, false
	}
}

func eventUserMessage(message agent.AgentMessage) (string, []llm.ImageContent, bool) {
	llmMessage, ok := agent.ToLLM(message)
	if !ok {
		return "", nil, false
	}
	user, ok := llmMessage.(*llm.UserMessage)
	if !ok {
		return "", nil, false
	}
	text, images := userMessageContent(user)
	return text, images, true
}

func addUsage(total *llm.Usage, usage llm.Usage) {
	total.Input += usage.Input
	total.Output += usage.Output
	total.CacheRead += usage.CacheRead
	total.CacheWrite += usage.CacheWrite
	tokens := usage.TotalTokens
	if tokens == 0 {
		tokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	}
	total.TotalTokens += tokens
	total.Cost.Input += usage.Cost.Input
	total.Cost.Output += usage.Cost.Output
	total.Cost.CacheRead += usage.Cost.CacheRead
	total.Cost.CacheWrite += usage.Cost.CacheWrite
	total.Cost.Total += usage.Cost.Total
}

func hasUsage(usage llm.Usage) bool {
	return usage.Input != 0 || usage.Output != 0 || usage.CacheRead != 0 ||
		usage.CacheWrite != 0 || usage.TotalTokens != 0 || usage.Cost.Total != 0
}

// eventAssistantText returns displayable assistant text. Failed messages retain
// their terminal state; aborted messages keep only content that actually
// streamed before the user stopped the run.
func eventAssistantText(message agent.AgentMessage) (string, bool) {
	assistant, ok := eventAssistantMessage(message)
	if !ok {
		return "", false
	}
	return displayAssistantText(assistant), true
}

func eventAssistantMessage(message agent.AgentMessage) (*llm.AssistantMessage, bool) {
	llmMessage, ok := agent.ToLLM(message)
	if !ok {
		return nil, false
	}
	assistant, ok := llmMessage.(*llm.AssistantMessage)
	if !ok || assistant == nil {
		return nil, false
	}
	return assistant, true
}

func displayAssistantText(assistant *llm.AssistantMessage) string {
	if assistant.StopReason == llm.StopReasonAborted {
		return assistant.Text()
	}
	if assistant.StopReason == llm.StopReasonError {
		if assistant.ErrorMessage != "" {
			return "[" + string(assistant.StopReason) + "] " + assistant.ErrorMessage
		}
		return "[" + string(assistant.StopReason) + "]"
	}
	return assistant.Text()
}

// eventToolResultText extracts text blocks from a tool result. Binary and
// structured blocks remain available to the lower-level agent but are omitted
// from the current text-oriented product shells.
func eventToolResultText(result any) string {
	toolResult, ok := result.(agent.ToolResult)
	if !ok {
		return ""
	}
	return toolResultContentText(toolResult.Content)
}

// eventToolResultDetails returns a tool's structured result, when it produced
// one. It is the source of truth product shells render; unlike Content it is not
// persisted, so it is available only on live events.
func eventToolResultDetails(result any) any {
	toolResult, ok := result.(agent.ToolResult)
	if !ok {
		return nil
	}
	return toolResult.Details
}
