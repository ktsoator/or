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
	// RunID is the durable lifecycle identity shared with local diagnostics.
	// It is populated for HistoryRun once run/start has been persisted.
	RunID string
	// MessageID is the durable transcript entry ID for persisted user and
	// assistant messages. Live messages remain empty until they are checkpointed.
	MessageID string
	// SentAt is the durable transcript timestamp for a persisted user message.
	SentAt time.Time

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
	// ToolOutcome is restored from the transcript's product-facing entries.
	ToolOutcome agent.ToolOutcome

	// Usage is populated for HistoryUsage and aggregates every assistant model
	// request that contributed to the preceding final response.
	Usage llm.Usage

	// Run timing is populated for HistoryRun. CompletedAt is also populated for
	// the final assistant response associated with a completed run.
	StartedAt   time.Time
	CompletedAt time.Time
}

// Messages returns every original message on the current transcript path. A
// compacted session therefore still exposes its complete history.
func (s *Session) Messages() []agent.AgentMessage {
	messages, err := s.journal.messagesSnapshot(s.agent.Snapshot().Messages)
	if err != nil {
		// Journal construction and every append validate the registered view.
		// Keep this snapshot-only API from returning a partial conversation if
		// that invariant is ever broken.
		return nil
	}
	return messages
}

// Entries returns a detached snapshot of the durable session log.
func (s *Session) Entries() []transcript.Entry {
	entries, _ := s.journal.entriesSnapshot()
	return entries
}

// History returns a displayable snapshot of the conversation in transcript
// order. The returned slice is detached from the agent's mutable state.
func (s *Session) History() []HistoryItem {
	activeRunID, activeStartedAt := s.lifecycle.activeRun()
	projection, persistedLen, err := s.journal.projectionSnapshot()
	if err != nil {
		// Loaded and engine-produced logs are validated before Session is exposed.
		// Returning no partial projection avoids presenting corrupt ownership as fact.
		return nil
	}
	outcomes := s.journal.outcomesSnapshot()

	active := s.agent.Snapshot().Messages
	var messages []agent.AgentMessage
	if persistedLen < len(active) {
		messages = active[persistedLen:]
	}
	items, foundActive := projectSessionHistory(projection, outcomes, activeRunID, messages)
	if activeRunID != "" && !foundActive {
		items = append(items, projectRecordedRunHistory(
			liveHistoryMessages(messages), outcomes, activeRunID, activeStartedAt, time.Time{},
		)...)
	} else if activeRunID == "" && len(messages) > 0 {
		items = append(items, projectHistory(messages, outcomes)...)
	}
	return items
}

func projectSessionHistory(
	projection *transcript.SessionProjection,
	outcomes map[string]agent.ToolOutcome,
	activeRunID string,
	live []agent.AgentMessage,
) ([]HistoryItem, bool) {
	var items []HistoryItem
	messagesByRun := make(map[string][]historyMessage, len(projection.Runs))
	for _, message := range projection.Messages {
		messagesByRun[message.RunID] = append(messagesByRun[message.RunID], historyMessage{
			message: message.Message, messageID: message.EntryID, timestamp: message.Timestamp,
		})
	}

	foundActive := false
	for _, run := range projection.Runs {
		messages := messagesByRun[run.ID]
		if run.ID == activeRunID {
			messages = append(messages, liveHistoryMessages(live)...)
			foundActive = true
		}
		items = append(items, projectRecordedRunHistory(
			messages, outcomes, run.ID, run.StartedAt, run.CompletedAt,
		)...)
	}
	return items, foundActive
}

func projectRecordedRunHistory(
	messages []historyMessage,
	outcomes map[string]agent.ToolOutcome,
	runID string,
	startedAt time.Time,
	completedAt time.Time,
) []HistoryItem {
	projected := projectHistoryMessages(messages, outcomes)
	if !completedAt.IsZero() {
		for index := len(projected) - 1; index >= 0; index-- {
			if projected[index].Type == HistoryAssistant && projected[index].FinalResponse {
				projected[index].CompletedAt = completedAt
				break
			}
		}
	}
	run := HistoryItem{Type: HistoryRun, RunID: runID, StartedAt: startedAt, CompletedAt: completedAt}
	if len(projected) > 0 && projected[0].Type == HistoryUser {
		items := make([]HistoryItem, 0, len(projected)+1)
		items = append(items, projected[0], run)
		return append(items, projected[1:]...)
	}
	return append([]HistoryItem{run}, projected...)
}

type historyMessage struct {
	message   agent.AgentMessage
	messageID string
	timestamp time.Time
}

func projectHistory(messages []agent.AgentMessage, outcomes map[string]agent.ToolOutcome) []HistoryItem {
	return projectHistoryMessages(liveHistoryMessages(messages), outcomes)
}

func liveHistoryMessages(messages []agent.AgentMessage) []historyMessage {
	result := make([]historyMessage, len(messages))
	for index, message := range messages {
		result[index] = historyMessage{message: message}
	}
	return result
}

func projectHistoryMessages(messages []historyMessage, outcomes map[string]agent.ToolOutcome) []HistoryItem {
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
	for _, recorded := range messages {
		llmMessage, ok := agent.ToLLM(recorded.message)
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
					Type:      HistoryUser,
					MessageID: recorded.messageID,
					SentAt:    recorded.timestamp,
					Text:      text,
					Images:    images,
					Files:     files,
				})
			}

		case *llm.AssistantMessage:
			addUsage(&usage, message.Usage)
			projected := assistantHistory(message)
			for index := range projected {
				if projected[index].Type == HistoryAssistant {
					projected[index].MessageID = recorded.messageID
				}
			}
			items = append(items, projected...)
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
				Images:      toolResultContentImages(message.Content),
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
	if message.StopReason == llm.StopReasonError {
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
			if block != nil && message.StopReason != llm.StopReasonAborted {
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
	if message.StopReason == llm.StopReasonAborted {
		// A terminal marker completes restored thinking without presenting an
		// unfinished response as final. Tool calls in an aborted provider payload
		// were never handed to the executor and are intentionally omitted above.
		if len(items) == 0 || items[len(items)-1].Type != HistoryAssistant {
			items = append(items, HistoryItem{
				Type: HistoryAssistant, Provider: message.Provider, Model: message.Model,
			})
		}
		return items
	}
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

func toolResultContentImages(contents []llm.ToolResultContent) []llm.ImageContent {
	var images []llm.ImageContent
	for _, content := range contents {
		if image, ok := content.(*llm.ImageContent); ok && image != nil {
			images = append(images, *image)
		}
	}
	return images
}
