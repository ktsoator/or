package coding

import (
	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// EventType identifies a UI-neutral coding-session event. Product adapters
// render these events for their own transport instead of depending on the
// lower-level agent event model.
type EventType string

const (
	RunStarted       EventType = "run_started"
	TextDelta        EventType = "text_delta"
	ThinkingDelta    EventType = "thinking_delta"
	ToolStarted      EventType = "tool_started"
	ToolFinished     EventType = "tool_finished"
	MessageCompleted EventType = "message_completed"
	RunCompleted     EventType = "run_completed"
)

// Event is the stable event contract exposed by Session. Fields are populated
// according to Type; presentation-specific concerns such as ANSI styling, JSON
// field names, SSE framing, and Markdown rendering stay in product adapters.
type Event struct {
	Type EventType

	// Streaming and completed assistant content.
	Delta string
	Text  string

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

	// Usage is the aggregate model consumption for a completed run. A run may
	// contain several assistant requests while tools execute; product shells
	// should present the aggregate once rather than exposing those internals.
	Usage llm.Usage
}

// projectAgentEvent maps a low-level agent event into the stable coding event
// contract. Agent events without product-visible meaning are omitted.
func projectAgentEvent(ev agent.AgentEvent) (Event, bool) {
	switch ev.Type {
	case agent.AgentStart:
		return Event{Type: RunStarted}, true

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
		text, ok := eventAssistantText(ev.Message)
		if !ok {
			return Event{}, false
		}
		return Event{Type: MessageCompleted, Text: text}, true

	case agent.AgentEnd:
		return Event{Type: RunCompleted, Usage: aggregateMessageUsage(ev.Messages)}, true

	default:
		return Event{}, false
	}
}

func aggregateMessageUsage(messages []agent.AgentMessage) llm.Usage {
	var total llm.Usage
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			continue
		}
		assistant, ok := llmMessage.(*llm.AssistantMessage)
		if !ok || assistant == nil {
			continue
		}
		addUsage(&total, assistant.Usage)
	}
	return total
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

// eventAssistantText returns displayable assistant text. Failed and aborted
// messages retain their terminal state in a compact textual form.
func eventAssistantText(message agent.AgentMessage) (string, bool) {
	llmMessage, ok := agent.ToLLM(message)
	if !ok {
		return "", false
	}
	assistant, ok := llmMessage.(*llm.AssistantMessage)
	if !ok {
		return "", false
	}
	if assistant.StopReason == llm.StopReasonError || assistant.StopReason == llm.StopReasonAborted {
		if assistant.ErrorMessage != "" {
			return "[" + string(assistant.StopReason) + "] " + assistant.ErrorMessage, true
		}
		return "[" + string(assistant.StopReason) + "]", true
	}
	return assistant.Text(), true
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
