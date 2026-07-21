// Package transcript defines the durable, append-only history of a coding
// session. The model-facing message list is a projection of these entries.
package transcript

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const CurrentVersion = 2

type EntryType string

const (
	MessageEntry    EntryType = "message"
	CompactionEntry EntryType = "compaction"
)

// Header is the first line of a v2 session log.
type Header struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

func NewHeader() Header { return Header{Type: "session", Version: CurrentVersion} }

// Entry is one node in the session history. ParentID makes the format ready for
// branching while the first version of coding continues to append linearly.
type Entry struct {
	ID         string
	ParentID   string
	Timestamp  time.Time
	Type       EntryType
	Message    agent.AgentMessage
	Compaction *Compaction
}

// Compaction records a summary boundary without deleting the entries it
// summarizes. FirstKeptEntryID points at the first original message retained in
// the active model context.
type Compaction struct {
	Summary           string    `json:"summary"`
	FirstKeptEntryID  string    `json:"firstKeptEntryId"`
	TokensBefore      int64     `json:"tokensBefore"`
	TokensAfter       int64     `json:"tokensAfter"`
	ReadFiles         []string  `json:"readFiles,omitempty"`
	ModifiedFiles     []string  `json:"modifiedFiles,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	Model             string    `json:"model,omitempty"`
	ResponseModel     string    `json:"responseModel,omitempty"`
	ResponseID        string    `json:"responseId,omitempty"`
	Usage             llm.Usage `json:"usage,omitempty"`
	ResponseTimestamp time.Time `json:"responseTimestamp,omitempty"`
}

func NewMessage(parentID string, message agent.AgentMessage) Entry {
	return Entry{
		ID:        NewID(),
		ParentID:  parentID,
		Timestamp: time.Now().UTC(),
		Type:      MessageEntry,
		Message:   message,
	}
}

func NewCompaction(parentID string, compact Compaction) Entry {
	return Entry{
		ID:         NewID(),
		ParentID:   parentID,
		Timestamp:  time.Now().UTC(),
		Type:       CompactionEntry,
		Compaction: &compact,
	}
}

func NewID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func (e Entry) Validate() error {
	if e.ID == "" {
		return errors.New("transcript: entry id is empty")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("transcript: entry %s timestamp is empty", e.ID)
	}
	switch e.Type {
	case MessageEntry:
		if e.Message == nil || e.Compaction != nil {
			return fmt.Errorf("transcript: message entry %s has invalid payload", e.ID)
		}
		if _, ok := agent.ToLLM(e.Message); !ok {
			return fmt.Errorf("transcript: cannot persist custom message %T", e.Message)
		}
	case CompactionEntry:
		if e.Message != nil || e.Compaction == nil {
			return fmt.Errorf("transcript: compaction entry %s has invalid payload", e.ID)
		}
		if e.Compaction.Summary == "" || e.Compaction.FirstKeptEntryID == "" {
			return fmt.Errorf("transcript: compaction entry %s is incomplete", e.ID)
		}
	default:
		return fmt.Errorf("transcript: entry %s has unknown type %q", e.ID, e.Type)
	}
	return nil
}

func (e Entry) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	wire := struct {
		ID         string          `json:"id"`
		ParentID   string          `json:"parentId,omitempty"`
		Timestamp  time.Time       `json:"timestamp"`
		Type       EntryType       `json:"type"`
		Message    json.RawMessage `json:"message,omitempty"`
		Compaction *Compaction     `json:"compaction,omitempty"`
	}{
		ID: e.ID, ParentID: e.ParentID, Timestamp: e.Timestamp, Type: e.Type,
		Compaction: e.Compaction,
	}
	if e.Message != nil {
		message, _ := agent.ToLLM(e.Message)
		encoded, err := llm.MarshalMessage(message)
		if err != nil {
			return nil, fmt.Errorf("transcript: encode message: %w", err)
		}
		wire.Message = encoded
	}
	return json.Marshal(wire)
}

func (e *Entry) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("transcript: decode into nil entry")
	}
	wire := struct {
		ID         string          `json:"id"`
		ParentID   string          `json:"parentId"`
		Timestamp  time.Time       `json:"timestamp"`
		Type       EntryType       `json:"type"`
		Message    json.RawMessage `json:"message"`
		Compaction *Compaction     `json:"compaction"`
	}{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	decoded := Entry{
		ID: wire.ID, ParentID: wire.ParentID, Timestamp: wire.Timestamp,
		Type: wire.Type, Compaction: wire.Compaction,
	}
	if len(wire.Message) > 0 && string(wire.Message) != "null" {
		message, err := llm.UnmarshalMessage(wire.Message)
		if err != nil {
			return fmt.Errorf("transcript: decode message: %w", err)
		}
		decoded.Message = agent.FromLLM(message)
	}
	if err := decoded.Validate(); err != nil {
		return err
	}
	*e = decoded
	return nil
}
