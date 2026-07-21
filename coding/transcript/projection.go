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

// BuildPath follows parent links from leaf to root and returns the entries in
// chronological order. An empty leaf selects the final appended entry.
func BuildPath(entries []Entry, leafID string) ([]Entry, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	byID := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		if _, exists := byID[entry.ID]; exists {
			return nil, fmt.Errorf("transcript: duplicate entry id %s", entry.ID)
		}
		byID[entry.ID] = entry
	}
	if leafID == "" {
		leafID = entries[len(entries)-1].ID
	}

	seen := make(map[string]bool, len(entries))
	reversed := make([]Entry, 0, len(entries))
	for leafID != "" {
		if seen[leafID] {
			return nil, fmt.Errorf("transcript: parent cycle at %s", leafID)
		}
		entry, ok := byID[leafID]
		if !ok {
			return nil, fmt.Errorf("transcript: missing entry %s", leafID)
		}
		seen[leafID] = true
		reversed = append(reversed, entry)
		leafID = entry.ParentID
	}

	path := make([]Entry, len(reversed))
	for index := range reversed {
		path[len(reversed)-1-index] = reversed[index]
	}
	return path, nil
}

// BuildContext projects a path into the messages sent to the model. Only the
// newest compaction boundary applies: its summary replaces the old prefix while
// original messages at and after FirstKeptEntryID remain verbatim.
func BuildContext(entries []Entry, leafID string) ([]agent.AgentMessage, error) {
	path, err := BuildPath(entries, leafID)
	if err != nil {
		return nil, err
	}
	latest := -1
	for index, entry := range path {
		if entry.Type == CompactionEntry {
			latest = index
		}
	}
	if latest < 0 {
		return messageEntries(path), nil
	}

	boundary := path[latest]
	result := []agent.AgentMessage{agent.UserMessage(
		summaryPrefix + boundary.Compaction.Summary + summarySuffix,
	)}
	foundFirstKept := false
	for index := 0; index < latest; index++ {
		entry := path[index]
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
	result = append(result, messageEntries(path[latest+1:])...)
	return result, nil
}

// Messages returns every original model message on the selected path. It omits
// compaction metadata and never exposes the synthetic summary message.
func Messages(entries []Entry, leafID string) ([]agent.AgentMessage, error) {
	path, err := BuildPath(entries, leafID)
	if err != nil {
		return nil, err
	}
	return messageEntries(path), nil
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
