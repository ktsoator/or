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
	HistoryUsage      HistoryItemType = "usage"
)

// HistoryItem is the displayable, product-neutral history contract exposed by
// Session. Product shells can render it without knowing the lower-level agent
// or LLM message representations.
type HistoryItem struct {
	Type HistoryItemType

	Text   string
	Images []llm.ImageContent
	// FinalResponse is true for the assistant item that completes one visible
	// reply. Tool-use pauses remain false even when they contain explanatory text.
	FinalResponse bool
	Provider      string
	Model         string

	ToolCallID string
	ToolName   string
	ToolArgs   any
	ToolResult string
	// ToolDetails is the tool's structured result (a tools.FileChange or
	// tools.MutationFailure) restored from the DetailsStore, when one was
	// persisted. It is nil when history replays as plain text.
	ToolDetails any
	IsError     bool

	// Usage is populated for HistoryUsage and aggregates every assistant model
	// request that contributed to the preceding final response.
	Usage llm.Usage
}

// History returns a displayable snapshot of the conversation in transcript
// order. The returned slice is detached from the agent's mutable state.
func (s *Session) History() []HistoryItem {
	return projectHistory(s.agent.Snapshot().Messages, s.snapshotDetails())
}

func projectHistory(messages []agent.AgentMessage, details map[string]any) []HistoryItem {
	var items []HistoryItem
	var usage llm.Usage
	flushUsage := func() {
		if !hasUsage(usage) {
			usage = llm.Usage{}
			return
		}
		items = append(items, HistoryItem{Type: HistoryUsage, Usage: usage})
		usage = llm.Usage{}
	}
	for _, message := range messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			continue
		}

		switch message := llmMessage.(type) {
		case *llm.UserMessage:
			// Steering messages may enter before the current visible reply is
			// complete, so pending tool-turn usage stays with the eventual response.
			// A normal/follow-up user message follows a final assistant, which has
			// already flushed its response usage below.
			text, images := userMessageContent(message)
			if text != "" || len(images) > 0 {
				items = append(items, HistoryItem{Type: HistoryUser, Text: text, Images: images})
			}

		case *llm.AssistantMessage:
			addUsage(&usage, message.Usage)
			items = append(items, assistantHistory(message)...)
			if message.StopReason != llm.StopReasonToolUse {
				flushUsage()
			}

		case *llm.ToolResultMessage:
			items = append(items, HistoryItem{
				Type:        HistoryToolResult,
				ToolCallID:  message.ToolCallID,
				ToolName:    message.ToolName,
				ToolResult:  toolResultContentText(message.Content),
				ToolDetails: details[message.ToolCallID],
				IsError:     message.IsError,
			})
		}
	}
	flushUsage()
	return items
}

func userMessageContent(message *llm.UserMessage) (string, []llm.ImageContent) {
	if message == nil {
		return "", nil
	}
	var text strings.Builder
	var images []llm.ImageContent
	for _, content := range message.Content {
		switch block := content.(type) {
		case *llm.TextContent:
			if block == nil {
				continue
			}
			text.WriteString(block.Text)
		case *llm.ImageContent:
			if block != nil {
				images = append(images, *block)
			}
		}
	}
	return text.String(), images
}

func assistantHistory(message *llm.AssistantMessage) []HistoryItem {
	if message == nil {
		return nil
	}
	if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
		text, _ := eventAssistantText(agent.FromLLM(message))
		return []HistoryItem{{
			Type:          HistoryAssistant,
			Text:          text,
			FinalResponse: true,
			Provider:      message.Provider,
			Model:         message.Model,
		}}
	}

	var items []HistoryItem
	var text strings.Builder
	var thinking strings.Builder
	flushText := func() {
		if text.Len() == 0 {
			return
		}
		items = append(items, HistoryItem{
			Type:     HistoryAssistant,
			Text:     text.String(),
			Provider: message.Provider,
			Model:    message.Model,
		})
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
	if message.StopReason != llm.StopReasonToolUse {
		for index := len(items) - 1; index >= 0; index-- {
			if items[index].Type == HistoryAssistant {
				items[index].FinalResponse = true
				break
			}
		}
	}
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
