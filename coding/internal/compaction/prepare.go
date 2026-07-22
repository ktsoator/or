package compaction

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

var ErrNothingToCompact = errors.New("coding: not enough history to compact")

type Prepared struct {
	Messages        []agent.AgentMessage
	PreviousSummary string
	FirstKeptID     string
	TokensBefore    int64
	KeptTokens      int64
}

// Prepare selects complete, oldest user turns for summarization. The retained
// suffix always begins with a user message, so tool calls and results cannot be
// separated from the turn that owns them.
func Prepare(entries []transcript.Entry, leafID string, keepRecentTokens int64) (Prepared, error) {
	path, err := transcript.BuildPath(entries, leafID)
	if err != nil {
		return Prepared{}, err
	}
	if keepRecentTokens <= 0 {
		return Prepared{}, fmt.Errorf("compaction: keep recent tokens must be positive")
	}

	active, previousSummary, err := activeMessageEntries(path)
	if err != nil {
		return Prepared{}, err
	}
	if len(active) < 2 {
		return Prepared{}, ErrNothingToCompact
	}

	starts := make([]int, 0, len(active))
	for index, entry := range active {
		message, _ := agent.ToLLM(entry.Message)
		if _, ok := message.(*llm.UserMessage); ok {
			starts = append(starts, index)
		}
	}
	if len(starts) < 2 {
		return Prepared{}, ErrNothingToCompact
	}

	boundary := starts[len(starts)-1]
	var keptTokens int64
	for turn := len(starts) - 1; turn >= 0; turn-- {
		start := starts[turn]
		end := len(active)
		if turn+1 < len(starts) {
			end = starts[turn+1]
		}
		keptTokens += EstimateEntries(active[start:end])
		boundary = start
		if keptTokens >= keepRecentTokens {
			break
		}
	}
	if boundary == 0 {
		return Prepared{}, ErrNothingToCompact
	}

	toSummarize := make([]agent.AgentMessage, 0, boundary)
	for _, entry := range active[:boundary] {
		toSummarize = append(toSummarize, entry.Message)
	}
	contextMessages, err := transcript.BuildContext(entries, leafID)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{
		Messages:        toSummarize,
		PreviousSummary: previousSummary,
		FirstKeptID:     active[boundary].ID,
		TokensBefore:    EstimateMessages(contextMessages),
		KeptTokens:      EstimateEntries(active[boundary:]),
	}, nil
}

func activeMessageEntries(path []transcript.Entry) ([]transcript.Entry, string, error) {
	latest := -1
	for index, entry := range path {
		if entry.Type == transcript.CompactionEntry {
			latest = index
		}
	}
	if latest < 0 {
		return onlyMessages(path), "", nil
	}

	boundary := path[latest]
	found := false
	active := make([]transcript.Entry, 0, len(path)-latest)
	for index := 0; index < latest; index++ {
		entry := path[index]
		if entry.ID == boundary.Compaction.FirstKeptEntryID {
			if entry.Type != transcript.MessageEntry {
				return nil, "", fmt.Errorf("compaction: first kept entry %s is not a message", entry.ID)
			}
			found = true
		}
		if found && entry.Type == transcript.MessageEntry {
			active = append(active, entry)
		}
	}
	if !found {
		return nil, "", fmt.Errorf("compaction: first kept entry %s was not found", boundary.Compaction.FirstKeptEntryID)
	}
	active = append(active, onlyMessages(path[latest+1:])...)
	return active, boundary.Compaction.Summary, nil
}

func onlyMessages(entries []transcript.Entry) []transcript.Entry {
	result := make([]transcript.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == transcript.MessageEntry {
			result = append(result, entry)
		}
	}
	return result
}

func EstimateEntries(entries []transcript.Entry) int64 {
	var tokens int64
	for _, entry := range entries {
		if entry.Type == transcript.MessageEntry {
			tokens += EstimateMessage(entry.Message)
		}
	}
	return tokens
}

func EstimateMessages(messages []agent.AgentMessage) int64 {
	var tokens int64
	for _, message := range messages {
		tokens += EstimateMessage(message)
	}
	return tokens
}

// EstimateMessage uses a conservative character heuristic. Provider usage is
// more accurate for thresholding later, but this is stable enough for choosing
// a complete-turn cut point.
func EstimateMessage(message agent.AgentMessage) int64 {
	value, ok := agent.ToLLM(message)
	if !ok {
		return 0
	}
	var chars int
	switch typed := value.(type) {
	case *llm.UserMessage:
		for _, content := range typed.Content {
			switch block := content.(type) {
			case *llm.TextContent:
				if block != nil {
					chars += len(block.Text)
				}
			case *llm.ImageContent:
				chars += 4800
			}
		}
	case *llm.AssistantMessage:
		for _, content := range typed.Content {
			switch block := content.(type) {
			case *llm.TextContent:
				if block != nil {
					chars += len(block.Text)
				}
			case *llm.ThinkingContent:
				if block != nil {
					chars += len(block.Thinking)
				}
			case *llm.ToolCall:
				if block != nil {
					encoded, _ := json.Marshal(block.Arguments)
					chars += len(block.Name) + len(encoded)
				}
			}
		}
	case *llm.ToolResultMessage:
		chars += len(typed.ToolName)
		for _, content := range typed.Content {
			switch block := content.(type) {
			case *llm.TextContent:
				if block != nil {
					chars += len(block.Text)
				}
			case *llm.ImageContent:
				chars += 4800
			}
		}
	}
	tokens := int64((chars + 3) / 4)
	if tokens < 4 {
		return 4
	}
	return tokens + 4
}
