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

const CurrentVersion = 3

type EntryType string

const (
	MessageEntry    EntryType = "message"
	CompactionEntry EntryType = "compaction"
	RunEntry        EntryType = "run"
)

// Header is the first line of a session log.
type Header struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

func NewHeader() Header { return Header{Type: "session", Version: CurrentVersion} }

// Entry is one item in the session's linear, append-only history.
type Entry struct {
	ID         string
	Timestamp  time.Time
	Type       EntryType
	Message    agent.AgentMessage
	Compaction *Compaction
	Run        *Run
}

// Run records the wall-clock interval for one agent invocation. FirstEntryID
// associates the timing with the messages appended by that run without adding
// product-only metadata to the model messages themselves.
type Run struct {
	FirstEntryID string    `json:"firstEntryId,omitempty"`
	StartedAt    time.Time `json:"startedAt"`
	CompletedAt  time.Time `json:"completedAt"`
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

func NewMessage(message agent.AgentMessage) Entry {
	return Entry{
		ID:        NewID(),
		Timestamp: time.Now().UTC(),
		Type:      MessageEntry,
		Message:   message,
	}
}

func NewCompaction(compact Compaction) Entry {
	return Entry{
		ID:         NewID(),
		Timestamp:  time.Now().UTC(),
		Type:       CompactionEntry,
		Compaction: &compact,
	}
}

func NewRun(firstEntryID string, startedAt, completedAt time.Time) Entry {
	return Entry{
		ID:        NewID(),
		Timestamp: completedAt.UTC(),
		Type:      RunEntry,
		Run: &Run{
			FirstEntryID: firstEntryID,
			StartedAt:    startedAt.UTC(),
			CompletedAt:  completedAt.UTC(),
		},
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
		if e.Message == nil || e.Compaction != nil || e.Run != nil {
			return fmt.Errorf("transcript: message entry %s has invalid payload", e.ID)
		}
		if _, ok := agent.ToLLM(e.Message); !ok {
			return fmt.Errorf("transcript: cannot persist custom message %T", e.Message)
		}
	case CompactionEntry:
		if e.Message != nil || e.Compaction == nil || e.Run != nil {
			return fmt.Errorf("transcript: compaction entry %s has invalid payload", e.ID)
		}
		if e.Compaction.Summary == "" || e.Compaction.FirstKeptEntryID == "" {
			return fmt.Errorf("transcript: compaction entry %s is incomplete", e.ID)
		}
	case RunEntry:
		if e.Message != nil || e.Compaction != nil || e.Run == nil {
			return fmt.Errorf("transcript: run entry %s has invalid payload", e.ID)
		}
		if e.Run.StartedAt.IsZero() || e.Run.CompletedAt.IsZero() {
			return fmt.Errorf("transcript: run entry %s is incomplete", e.ID)
		}
		if e.Run.CompletedAt.Before(e.Run.StartedAt) {
			return fmt.Errorf("transcript: run entry %s completes before it starts", e.ID)
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
		Timestamp  time.Time       `json:"timestamp"`
		Type       EntryType       `json:"type"`
		Message    json.RawMessage `json:"message,omitempty"`
		Compaction *Compaction     `json:"compaction,omitempty"`
		Run        *Run            `json:"run,omitempty"`
	}{
		ID: e.ID, Timestamp: e.Timestamp, Type: e.Type,
		Compaction: e.Compaction, Run: e.Run,
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
		Timestamp  time.Time       `json:"timestamp"`
		Type       EntryType       `json:"type"`
		Message    json.RawMessage `json:"message"`
		Compaction *Compaction     `json:"compaction"`
		Run        *Run            `json:"run"`
	}{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	decoded := Entry{
		ID: wire.ID, Timestamp: wire.Timestamp,
		Type: wire.Type, Compaction: wire.Compaction, Run: wire.Run,
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
