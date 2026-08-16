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

const CurrentVersion = 5

type EntryType string

const (
	MessageEntry     EntryType = "message"
	ToolCallEntry    EntryType = "tool_call"
	ToolOutcomeEntry EntryType = "tool_outcome"
	ContextEntry     EntryType = "context"
	CompactionEntry  EntryType = "compaction"
	RunStartEntry    EntryType = "run/start"
	RunEndEntry      EntryType = "run/end"
	TurnStartEntry   EntryType = "turn/start"
	TurnEndEntry     EntryType = "turn/end"
	StepStartEntry   EntryType = "step/start"
	StepEndEntry     EntryType = "step/end"
)

type LifecycleStatus string

const (
	LifecycleCompleted   LifecycleStatus = "completed"
	LifecycleFailed      LifecycleStatus = "failed"
	LifecycleCancelled   LifecycleStatus = "cancelled"
	LifecycleInterrupted LifecycleStatus = "interrupted"
)

// Header is the first line of a session log.
type Header struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

func NewHeader() Header { return Header{Type: "session", Version: CurrentVersion} }

// Entry is one item in the session's linear, append-only history.
type Entry struct {
	ID          string
	Timestamp   time.Time
	Type        EntryType
	Message     agent.AgentMessage
	ToolCall    *ToolCall
	ToolOutcome *ToolOutcome
	Context     *ContextAttachment
	Compaction  *Compaction
	Lifecycle   *Lifecycle
}

// Lifecycle identifies one durable Run, Turn, or Step boundary. Entry.Type
// supplies the boundary kind; parent IDs make ownership explicit and stable.
type Lifecycle struct {
	RunID  string          `json:"runId"`
	TurnID string          `json:"turnId,omitempty"`
	StepID string          `json:"stepId,omitempty"`
	Status LifecycleStatus `json:"status,omitempty"`
	Reason string          `json:"reason,omitempty"`
}

// ToolCall is a durable dispatch intent. Its presence means validation and
// authorization completed and the tool body may have started. Arguments are
// the normalized JSON value passed to the tool, not a presentation summary.
type ToolCall struct {
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Arguments  json.RawMessage `json:"arguments"`
}

// ToolOutcome records the product-facing result associated with one model-
// visible tool result. Data stays provider-neutral and is decoded by the engine
// according to DataKind.
type ToolOutcome struct {
	ToolCallID string                  `json:"toolCallId"`
	Status     agent.ToolOutcomeStatus `json:"status"`
	ErrorCode  string                  `json:"errorCode,omitempty"`
	ExitCode   *int                    `json:"exitCode,omitempty"`
	DataKind   string                  `json:"dataKind,omitempty"`
	Data       json.RawMessage         `json:"data,omitempty"`
}

// ContextAttachment records one product-generated model-context block without
// representing it as a user-authored conversation message. Epoch increments
// when a session process rebuilds its context snapshot. Placement describes how
// the model-input projector positions the rendered block.
type ContextAttachment struct {
	AttachmentID string `json:"attachmentId"`
	Epoch        uint64 `json:"epoch"`
	Kind         string `json:"kind"`
	Placement    string `json:"placement"`
	Path         string `json:"path,omitempty"`
	Revision     string `json:"revision"`
	Rendered     string `json:"rendered"`
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

func NewToolCall(call ToolCall) Entry {
	return Entry{
		ID:        NewID(),
		Timestamp: time.Now().UTC(),
		Type:      ToolCallEntry,
		ToolCall:  &call,
	}
}

func NewToolOutcome(outcome ToolOutcome) Entry {
	return Entry{
		ID:          NewID(),
		Timestamp:   time.Now().UTC(),
		Type:        ToolOutcomeEntry,
		ToolOutcome: &outcome,
	}
}

func NewContext(context ContextAttachment) Entry {
	return Entry{
		ID:        NewID(),
		Timestamp: time.Now().UTC(),
		Type:      ContextEntry,
		Context:   &context,
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

func NewRunStart(runID string) Entry {
	return newLifecycleEntry(RunStartEntry, Lifecycle{RunID: runID})
}

func NewRunEnd(runID string, status LifecycleStatus, reason string) Entry {
	return newLifecycleEntry(RunEndEntry, Lifecycle{
		RunID: runID, Status: status, Reason: reason,
	})
}

func NewTurnStart(runID, turnID string) Entry {
	return newLifecycleEntry(TurnStartEntry, Lifecycle{RunID: runID, TurnID: turnID})
}

func NewTurnEnd(runID, turnID string, status LifecycleStatus, reason string) Entry {
	return newLifecycleEntry(TurnEndEntry, Lifecycle{
		RunID: runID, TurnID: turnID, Status: status, Reason: reason,
	})
}

func NewStepStart(runID, turnID, stepID string) Entry {
	return newLifecycleEntry(StepStartEntry, Lifecycle{
		RunID: runID, TurnID: turnID, StepID: stepID,
	})
}

func NewStepEnd(
	runID, turnID, stepID string,
	status LifecycleStatus,
	reason string,
) Entry {
	return newLifecycleEntry(StepEndEntry, Lifecycle{
		RunID: runID, TurnID: turnID, StepID: stepID,
		Status: status, Reason: reason,
	})
}

func newLifecycleEntry(entryType EntryType, lifecycle Lifecycle) Entry {
	return Entry{
		ID: NewID(), Timestamp: time.Now().UTC(), Type: entryType,
		Lifecycle: &lifecycle,
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
		if e.Message == nil || e.ToolCall != nil || e.ToolOutcome != nil || e.Context != nil || e.Compaction != nil || e.Lifecycle != nil {
			return fmt.Errorf("transcript: message entry %s has invalid payload", e.ID)
		}
		if _, ok := agent.ToLLM(e.Message); !ok {
			return fmt.Errorf("transcript: cannot persist custom message %T", e.Message)
		}
	case ToolCallEntry:
		if e.Message != nil || e.ToolCall == nil || e.ToolOutcome != nil || e.Context != nil || e.Compaction != nil || e.Lifecycle != nil {
			return fmt.Errorf("transcript: tool call entry %s has invalid payload", e.ID)
		}
		if e.ToolCall.ToolCallID == "" || e.ToolCall.ToolName == "" ||
			len(e.ToolCall.Arguments) == 0 {
			return fmt.Errorf("transcript: tool call entry %s is incomplete", e.ID)
		}
		if !json.Valid(e.ToolCall.Arguments) {
			return fmt.Errorf("transcript: tool call entry %s has invalid arguments", e.ID)
		}
	case ToolOutcomeEntry:
		if e.Message != nil || e.ToolCall != nil || e.ToolOutcome == nil || e.Context != nil || e.Compaction != nil || e.Lifecycle != nil {
			return fmt.Errorf("transcript: tool outcome entry %s has invalid payload", e.ID)
		}
		if e.ToolOutcome.ToolCallID == "" || e.ToolOutcome.Status == "" {
			return fmt.Errorf("transcript: tool outcome entry %s is incomplete", e.ID)
		}
		if len(e.ToolOutcome.Data) > 0 && !json.Valid(e.ToolOutcome.Data) {
			return fmt.Errorf("transcript: tool outcome entry %s has invalid data", e.ID)
		}
	case ContextEntry:
		if e.Message != nil || e.ToolCall != nil || e.ToolOutcome != nil || e.Context == nil || e.Compaction != nil || e.Lifecycle != nil {
			return fmt.Errorf("transcript: context entry %s has invalid payload", e.ID)
		}
		if e.Context.AttachmentID == "" ||
			e.Context.Epoch == 0 ||
			e.Context.Kind == "" ||
			e.Context.Placement == "" ||
			e.Context.Revision == "" ||
			e.Context.Rendered == "" {
			return fmt.Errorf("transcript: context entry %s is incomplete", e.ID)
		}
	case CompactionEntry:
		if e.Message != nil || e.ToolCall != nil || e.ToolOutcome != nil || e.Context != nil || e.Compaction == nil || e.Lifecycle != nil {
			return fmt.Errorf("transcript: compaction entry %s has invalid payload", e.ID)
		}
		if e.Compaction.Summary == "" || e.Compaction.FirstKeptEntryID == "" {
			return fmt.Errorf("transcript: compaction entry %s is incomplete", e.ID)
		}
	case RunStartEntry, RunEndEntry,
		TurnStartEntry, TurnEndEntry,
		StepStartEntry, StepEndEntry:
		if e.Message != nil || e.ToolCall != nil || e.ToolOutcome != nil || e.Context != nil || e.Compaction != nil || e.Lifecycle == nil {
			return fmt.Errorf("transcript: lifecycle entry %s has invalid payload", e.ID)
		}
		if err := validateLifecyclePayload(e.Type, *e.Lifecycle); err != nil {
			return fmt.Errorf("transcript: lifecycle entry %s: %w", e.ID, err)
		}
	default:
		return fmt.Errorf("transcript: entry %s has unknown type %q", e.ID, e.Type)
	}
	return nil
}

func validateLifecyclePayload(entryType EntryType, lifecycle Lifecycle) error {
	if lifecycle.RunID == "" {
		return errors.New("run id is empty")
	}
	switch entryType {
	case RunStartEntry, RunEndEntry:
		if lifecycle.TurnID != "" || lifecycle.StepID != "" {
			return errors.New("run boundary has child ids")
		}
	case TurnStartEntry, TurnEndEntry:
		if lifecycle.TurnID == "" || lifecycle.StepID != "" {
			return errors.New("turn boundary has invalid child ids")
		}
	case StepStartEntry, StepEndEntry:
		if lifecycle.TurnID == "" || lifecycle.StepID == "" {
			return errors.New("step boundary has incomplete ids")
		}
	}
	isEnd := entryType == RunEndEntry || entryType == TurnEndEntry || entryType == StepEndEntry
	if !isEnd {
		if lifecycle.Status != "" || lifecycle.Reason != "" {
			return errors.New("start boundary has terminal fields")
		}
		return nil
	}
	switch lifecycle.Status {
	case LifecycleCompleted, LifecycleFailed, LifecycleCancelled, LifecycleInterrupted:
		return nil
	default:
		return fmt.Errorf("invalid terminal status %q", lifecycle.Status)
	}
}

func (e Entry) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	wire := struct {
		ID          string             `json:"id"`
		Timestamp   time.Time          `json:"timestamp"`
		Type        EntryType          `json:"type"`
		Message     json.RawMessage    `json:"message,omitempty"`
		ToolCall    *ToolCall          `json:"toolCall,omitempty"`
		ToolOutcome *ToolOutcome       `json:"toolOutcome,omitempty"`
		Context     *ContextAttachment `json:"context,omitempty"`
		Compaction  *Compaction        `json:"compaction,omitempty"`
		Lifecycle   *Lifecycle         `json:"lifecycle,omitempty"`
	}{
		ID: e.ID, Timestamp: e.Timestamp, Type: e.Type,
		ToolCall: e.ToolCall, ToolOutcome: e.ToolOutcome, Context: e.Context,
		Compaction: e.Compaction, Lifecycle: e.Lifecycle,
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
		ID          string             `json:"id"`
		Timestamp   time.Time          `json:"timestamp"`
		Type        EntryType          `json:"type"`
		Message     json.RawMessage    `json:"message"`
		ToolCall    *ToolCall          `json:"toolCall"`
		ToolOutcome *ToolOutcome       `json:"toolOutcome"`
		Context     *ContextAttachment `json:"context"`
		Compaction  *Compaction        `json:"compaction"`
		Lifecycle   *Lifecycle         `json:"lifecycle"`
	}{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	decoded := Entry{
		ID: wire.ID, Timestamp: wire.Timestamp,
		Type: wire.Type, ToolCall: wire.ToolCall, ToolOutcome: wire.ToolOutcome, Context: wire.Context,
		Compaction: wire.Compaction, Lifecycle: wire.Lifecycle,
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
