package transcript

import (
	"fmt"

	"github.com/ktsoator/or/agent"
)

const ModelContextProjectionKey = "model-context"

// ModelContextProjection is the active canonical message context at one
// committed transcript sequence. Product-generated context attachments are
// projected separately by the engine at provider-request time.
type ModelContextProjection struct {
	AppliedEntries          int
	AsOfSeq                 int64
	Messages                []agent.AgentMessage
	ActiveCompactionEntryID string
	FirstKeptEntryID        string
}

// ModelContextProjectionUnit incrementally maintains the same canonical
// message view produced by BuildContext. It retains original messages so a
// later compaction can select any validated preceding message as its boundary.
type ModelContextProjectionUnit struct {
	projection   ModelContextProjection
	allMessages  []agent.AgentMessage
	messageIndex map[string]int
}

func NewModelContextProjectionUnit() *ModelContextProjectionUnit {
	return &ModelContextProjectionUnit{
		projection:   ModelContextProjection{AsOfSeq: -1},
		messageIndex: make(map[string]int),
	}
}

func (*ModelContextProjectionUnit) ProjectionKey() string {
	return ModelContextProjectionKey
}

func (u *ModelContextProjectionUnit) ApplyProjection(event ProjectionEvent) {
	switch event.Entry.Type {
	case MessageEntry:
		u.messageIndex[event.Entry.ID] = len(u.allMessages)
		u.allMessages = append(u.allMessages, event.message)
		u.projection.Messages = append(u.projection.Messages, event.message)

	case CompactionEntry:
		first, ok := u.messageIndex[event.Entry.Compaction.FirstKeptEntryID]
		if !ok {
			panic(fmt.Sprintf(
				"transcript: validated compaction %s references unknown message %s",
				event.Entry.ID,
				event.Entry.Compaction.FirstKeptEntryID,
			))
		}
		active := make([]agent.AgentMessage, 1, 1+len(u.allMessages)-first)
		active[0] = agent.UserMessage(
			summaryPrefix + event.Entry.Compaction.Summary + summarySuffix,
		)
		for _, message := range u.allMessages[first:] {
			active = append(active, message)
		}
		u.projection.Messages = active
		u.projection.ActiveCompactionEntryID = event.Entry.ID
		u.projection.FirstKeptEntryID = event.Entry.Compaction.FirstKeptEntryID
	}

	u.projection.AppliedEntries = event.EntryIndex + 1
	u.projection.AsOfSeq = event.Entry.Seq
}

func (u *ModelContextProjectionUnit) SnapshotProjection() (any, error) {
	return u.Snapshot()
}

func (u *ModelContextProjectionUnit) Snapshot() (*ModelContextProjection, error) {
	if u == nil {
		return nil, fmt.Errorf("transcript: model-context projection unit is nil")
	}
	clone := u.projection
	clone.Messages = make([]agent.AgentMessage, len(u.projection.Messages))
	for index, message := range u.projection.Messages {
		llmMessage, ok := agent.ToLLM(message)
		if !ok {
			return nil, fmt.Errorf(
				"transcript: model-context message %d is not model-facing",
				index,
			)
		}
		detached, err := cloneProjectedMessage(llmMessage)
		if err != nil {
			return nil, fmt.Errorf(
				"transcript: clone model-context message %d: %w",
				index,
				err,
			)
		}
		clone.Messages[index] = detached
	}
	return &clone, nil
}

// ProjectModelContext replays a complete committed transcript through the
// same projection unit used by live sessions.
func ProjectModelContext(entries []Entry) (*ModelContextProjection, error) {
	registry := NewProjectionRegistry()
	unit := NewModelContextProjectionUnit()
	if err := registry.Register(unit); err != nil {
		return nil, err
	}
	if _, err := validateSession(entries, registry); err != nil {
		return nil, err
	}
	return unit.Snapshot()
}
