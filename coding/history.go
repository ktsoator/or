package coding

import (
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// HistoryItemType identifies one UI-neutral item reconstructed from the
// persisted conversation transcript.
type HistoryItemType string

const (
	HistoryUser       HistoryItemType = "user"
	HistoryAssistant  HistoryItemType = "assistant"
	HistoryThinking   HistoryItemType = "thinking"
	HistoryToolCall   HistoryItemType = "tool_call"
	HistoryToolResult HistoryItemType = "tool_result"
)

// HistoryItem is the displayable, product-neutral history contract exposed by
// Session. Product shells can render it without knowing the lower-level agent
// or LLM message representations.
type HistoryItem struct {
	Type HistoryItemType

	Text string

	ToolCallID string
	ToolName   string
	ToolArgs   any
	ToolResult string
	IsError    bool
}

// History returns a displayable snapshot of the conversation in transcript
// order. The returned slice is detached from the agent's mutable state.
func (s *Session) History() []HistoryItem {
	return projectHistory(s.agent.Snapshot().Messages)
}

func projectHistory(messages []agent.AgentMessage) []HistoryItem {
	var items []HistoryItem
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			continue
		}

		switch message := llmMessage.(type) {
		case *llm.UserMessage:
			if text := userMessageText(message); text != "" {
				items = append(items, HistoryItem{Type: HistoryUser, Text: text})
			}

		case *llm.AssistantMessage:
			items = append(items, assistantHistory(message)...)

		case *llm.ToolResultMessage:
			items = append(items, HistoryItem{
				Type:       HistoryToolResult,
				ToolCallID: message.ToolCallID,
				ToolName:   message.ToolName,
				ToolResult: toolResultContentText(message.Content),
				IsError:    message.IsError,
			})
		}
	}
	return items
}

func userMessageText(message *llm.UserMessage) string {
	if message == nil {
		return ""
	}
	var text strings.Builder
	for _, content := range message.Content {
		if block, ok := content.(*llm.TextContent); ok && block != nil {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

func assistantHistory(message *llm.AssistantMessage) []HistoryItem {
	if message == nil {
		return nil
	}
	if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
		text, _ := eventAssistantText(agent.FromLLM(message))
		return []HistoryItem{{Type: HistoryAssistant, Text: text}}
	}

	var items []HistoryItem
	var text strings.Builder
	var thinking strings.Builder
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		items = append(items, HistoryItem{Type: HistoryAssistant, Text: text.String()})
		text.Reset()
	}
	flushThinking := func() {
		if thinking.Len() == 0 {
			return
		}
		items = append(items, HistoryItem{Type: HistoryThinking, Text: thinking.String()})
		thinking.Reset()
	}

	for _, content := range message.Content {
		switch block := content.(type) {
		case *llm.TextContent:
			flushThinking()
			if block != nil {
				text.WriteString(block.Text)
			}

		case *llm.ThinkingContent:
			flushText()
			if block != nil && !block.Redacted {
				thinking.WriteString(block.Thinking)
			}

		case *llm.ToolCall:
			flushText()
			flushThinking()
			if block != nil {
				items = append(items, HistoryItem{
					Type:       HistoryToolCall,
					ToolCallID: block.ID,
					ToolName:   block.Name,
					ToolArgs:   block.Arguments,
				})
			}
		}
	}
	flushText()
	flushThinking()
	return items
}

func toolResultContentText(contents []llm.ToolResultContent) string {
	var parts []string
	for _, content := range contents {
		if text, ok := content.(*llm.TextContent); ok && text != nil {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
