package engine

import (
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/transcript"
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
	Files  []File
	// FinalResponse is true for the assistant item that completes one visible
	// reply. Tool-use pauses remain false even when they contain explanatory text.
	FinalResponse bool
	Provider      string
	Model         string

	ToolCallID string
	ToolName   string
	ToolArgs   any
	ToolResult string
	// ToolOutcome is restored from the sidecar.
	ToolOutcome agent.ToolOutcome

	// Usage is populated for HistoryUsage and aggregates every assistant model
	// request that contributed to the preceding final response.
	Usage llm.Usage

	// Run timing is populated for HistoryRun. CompletedAt is also populated for
	// the final assistant response associated with a completed run.
	StartedAt   time.Time
	CompletedAt time.Time
}

// History returns a displayable snapshot of the conversation in transcript
// order. The returned slice is detached from the agent's mutable state.
func (s *Session) History() []HistoryItem {
	_, activeStartedAt, activeEntryStart := s.activeRunState()
	entries, persistedLen := s.snapshotTranscriptState()
	outcomes := s.snapshotOutcomes()

	active := s.agent.Snapshot().Messages
	var messages []agent.AgentMessage
	if persistedLen < len(active) {
		messages = active[persistedLen:]
	}
	if !activeStartedAt.IsZero() {
		// persistNewRun writes the completed Run entry before RunCompleted is
		// dispatched. A concurrent history request in that short window must not
		// append a second, apparently still-open run.
		if containsRunStartedAt(entries, activeStartedAt) {
			items := projectEntryHistory(entries, outcomes)
			if len(messages) > 0 {
				items = append(items, projectHistory(messages, outcomes)...)
			}
			return items
		}
		firstEntryID := firstMessageFrom(entries, activeEntryStart)
		if firstEntryID != "" {
			entries = append(entries, transcript.Entry{
				Type: transcript.RunEntry,
				Run:  &transcript.Run{FirstEntryID: firstEntryID, StartedAt: activeStartedAt},
			})
			items := projectEntryHistory(entries, outcomes)
			return append(items, projectHistory(messages, outcomes)...)
		}
		items := projectEntryHistory(entries, outcomes)
		return append(items, projectRunHistory(messages, outcomes, activeStartedAt, time.Time{})...)
	}

	items := projectEntryHistory(entries, outcomes)
	if len(messages) > 0 {
		items = append(items, projectHistory(messages, outcomes)...)
	}
	return items
}

func containsRunStartedAt(entries []transcript.Entry, startedAt time.Time) bool {
	for _, entry := range entries {
		if entry.Type == transcript.RunEntry && entry.Run != nil && entry.Run.StartedAt.Equal(startedAt) {
			return true
		}
	}
	return false
}

func projectEntryHistory(entries []transcript.Entry, outcomes map[string]agent.ToolOutcome) []HistoryItem {
	var items []HistoryItem
	var pending []transcript.Entry
	flushMessages := func(entries []transcript.Entry) {
		items = append(items, projectHistory(entryMessages(entries), outcomes)...)
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
			items = append(items, projectRecordedRunHistory(
				entryMessages(pending[first:]), outcomes, entry.Run.StartedAt, entry.Run.CompletedAt,
			)...)
			pending = nil
		}
	}
	flushMessages(pending)
	return items
}

func projectRunHistory(
	messages []agent.AgentMessage,
	outcomes map[string]agent.ToolOutcome,
	startedAt time.Time,
	completedAt time.Time,
) []HistoryItem {
	return projectRecordedRunHistory(messages, outcomes, startedAt, completedAt)
}

func projectRecordedRunHistory(
	messages []agent.AgentMessage,
	outcomes map[string]agent.ToolOutcome,
	startedAt time.Time,
	completedAt time.Time,
) []HistoryItem {
	projected := projectHistory(messages, outcomes)
	if !completedAt.IsZero() {
		for index := len(projected) - 1; index >= 0; index-- {
			if projected[index].Type == HistoryAssistant && projected[index].FinalResponse {
				projected[index].CompletedAt = completedAt
				break
			}
		}
	}
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

func projectHistory(messages []agent.AgentMessage, outcomes map[string]agent.ToolOutcome) []HistoryItem {
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
	for _, agentMessage := range messages {
		llmMessage, ok := agent.ToLLM(agentMessage)
		if !ok {
			continue
		}

		switch message := llmMessage.(type) {
		case *llm.UserMessage:
			// Steering messages may enter before the current visible reply is
			// complete, so pending tool-turn usage stays with the eventual response.
			// A normal/follow-up user message follows a final assistant, which has
			// already flushed its response usage below.
			text, images, files := userMessageContent(message)
			if text != "" || len(images) > 0 || len(files) > 0 {
				items = append(items, HistoryItem{
					Type:   HistoryUser,
					Text:   text,
					Images: images,
					Files:  files,
				})
			}

		case *llm.AssistantMessage:
			addUsage(&usage, message.Usage)
			items = append(items, assistantHistory(message)...)
			if message.StopReason != llm.StopReasonToolUse {
				flushUsage()
			}

		case *llm.ToolResultMessage:
			outcome, ok := outcomes[message.ToolCallID]
			if !ok {
				outcome = agent.ToolOutcome{Status: agent.ToolOutcomeSuccess}
			}
			items = append(items, HistoryItem{
				Type:        HistoryToolResult,
				ToolCallID:  message.ToolCallID,
				ToolName:    message.ToolName,
				ToolResult:  toolResultContentText(message.Content),
				ToolOutcome: outcome,
			})
		}
	}
	flushUsage()
	return items
}

func userMessageContent(message *llm.UserMessage) (string, []llm.ImageContent, []File) {
	if message == nil {
		return "", nil, nil
	}
	var text strings.Builder
	var images []llm.ImageContent
	var files []File
	textBlocks := 0
	for _, content := range message.Content {
		switch block := content.(type) {
		case *llm.TextContent:
			if block == nil {
				continue
			}
			if textBlocks > 0 {
				if attached, matched := parseAttachedFilesContext(block.Text); matched {
					files = append(files, attached...)
					textBlocks++
					continue
				}
			}
			if textBlocks > 0 && skills.IsExplicitInvocationText(block.Text) {
				textBlocks++
				continue
			}
			textBlocks++
			text.WriteString(block.Text)
		case *llm.ImageContent:
			if block != nil {
				images = append(images, *block)
			}
		}
	}
	return text.String(), images, files
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
