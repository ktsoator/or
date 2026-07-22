package coding

import (
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/transcript"
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
	HistoryRun        HistoryItemType = "run"
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

	// Run timing is populated for HistoryRun. CompletedAt stays zero while a
	// history snapshot is taken during the active run.
	StartedAt   time.Time
	CompletedAt time.Time
}

// History returns a displayable snapshot of the conversation in transcript
// order. The returned slice is detached from the agent's mutable state.
func (s *Session) History() []HistoryItem {
	activeStartedAt := s.activeRunStartedAt()
	entries, leafID, persistedLen := s.snapshotTranscriptState()
	details := s.snapshotDetails()
	path, err := transcript.BuildPath(entries, leafID)
	if err != nil {
		return projectHistory(s.Messages(), details)
	}
	items := projectEntryHistory(path, details)

	active := s.agent.Snapshot().Messages
	var messages []agent.AgentMessage
	if persistedLen < len(active) {
		messages = active[persistedLen:]
	}
	if !activeStartedAt.IsZero() {
		persistedRun := false
		for _, item := range items {
			if item.Type == HistoryRun && item.StartedAt.Equal(activeStartedAt) {
				persistedRun = true
				break
			}
		}
		if !persistedRun {
			items = append(items, projectRunHistory(messages, details, activeStartedAt, time.Time{})...)
		}
	} else if len(messages) > 0 {
		items = append(items, projectHistory(messages, details)...)
	}
	return items
}

func projectEntryHistory(entries []transcript.Entry, details map[string]any) []HistoryItem {
	var items []HistoryItem
	var pending []transcript.Entry
	flushMessages := func(entries []transcript.Entry) {
		items = append(items, projectHistory(entryMessages(entries), details)...)
	}

	for _, entry := range entries {
		switch entry.Type {
		case transcript.MessageEntry:
			pending = append(pending, entry)
		case transcript.RunEntry:
			first := -1
			if entry.Run != nil && entry.Run.FirstEntryID != "" {
				for index := range pending {
					if pending[index].ID == entry.Run.FirstEntryID {
						first = index
						break
					}
				}
			}
			if first < 0 {
				flushMessages(pending)
				pending = nil
				if entry.Run != nil {
					items = append(items, HistoryItem{
						Type: HistoryRun, StartedAt: entry.Run.StartedAt, CompletedAt: entry.Run.CompletedAt,
					})
				}
				continue
			}
			flushMessages(pending[:first])
			items = append(items, projectRunHistory(
				entryMessages(pending[first:]), details, entry.Run.StartedAt, entry.Run.CompletedAt,
			)...)
			pending = nil
		}
	}
	flushMessages(pending)
	return items
}

func projectRunHistory(
	messages []agent.AgentMessage,
	details map[string]any,
	startedAt time.Time,
	completedAt time.Time,
) []HistoryItem {
	projected := projectHistory(messages, details)
	run := HistoryItem{Type: HistoryRun, StartedAt: startedAt, CompletedAt: completedAt}
	if len(projected) > 0 && projected[0].Type == HistoryUser {
		items := make([]HistoryItem, 0, len(projected)+1)
		items = append(items, projected[0], run)
		return append(items, projected[1:]...)
	}
	return append([]HistoryItem{run}, projected...)
}

func entryMessages(entries []transcript.Entry) []agent.AgentMessage {
	messages := make([]agent.AgentMessage, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == transcript.MessageEntry {
			messages = append(messages, entry.Message)
		}
	}
	return messages
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
