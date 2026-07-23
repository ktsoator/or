package transcript

import (
	"fmt"

	"github.com/ktsoator/or/agent"
)

const summaryPrefix = `The conversation history before this point was compacted into the following summary:

<summary>
`

const summarySuffix = `
</summary>`

// BuildContext projects the linear log into the messages sent to the model.
// Only the newest compaction boundary applies: its summary replaces the old
// prefix while original messages at and after FirstKeptEntryID remain verbatim.
func BuildContext(entries []Entry) ([]agent.AgentMessage, error) {
	latest := -1
	for index, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		if entry.Type == CompactionEntry {
			latest = index
		}
	}
	if latest < 0 {
		return messageEntries(entries), nil
	}

	boundary := entries[latest]
	result := []agent.AgentMessage{agent.UserMessage(
		summaryPrefix + boundary.Compaction.Summary + summarySuffix,
	)}
	foundFirstKept := false
	for index := 0; index < latest; index++ {
		entry := entries[index]
		if entry.ID == boundary.Compaction.FirstKeptEntryID {
			if entry.Type != MessageEntry {
				return nil, fmt.Errorf("transcript: first kept entry %s is not a message", entry.ID)
			}
			foundFirstKept = true
		}
		if foundFirstKept && entry.Type == MessageEntry {
			result = append(result, entry.Message)
		}
	}
	if !foundFirstKept {
		return nil, fmt.Errorf(
			"transcript: first kept entry %s is not before compaction %s",
			boundary.Compaction.FirstKeptEntryID, boundary.ID,
		)
	}
	result = append(result, messageEntries(entries[latest+1:])...)
	return result, nil
}

// Messages returns every original model message in the log. It omits
// compaction metadata and never exposes the synthetic summary message.
func Messages(entries []Entry) []agent.AgentMessage {
	return messageEntries(entries)
}

func messageEntries(entries []Entry) []agent.AgentMessage {
	messages := make([]agent.AgentMessage, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == MessageEntry {
			messages = append(messages, entry.Message)
		}
	}
	return messages
}
