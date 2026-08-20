package transcript

import (
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
)

// ProjectionEvent is one validated event at its committed transcript
// position. Entry and Scope are immutable inputs to registered projections.
type ProjectionEvent struct {
	Entry      Entry
	EntryIndex int
	Scope      ProjectedLifecycle

	message      agent.AgentMessage
	toolRequests []projectedToolRequest
	toolResultID string
}

type projectedToolRequest struct {
	ID                  string
	Name                string
	Arguments           []byte
	Scope               ProjectedLifecycle
	AssistantEntryID    string
	AssistantEntryIndex int
}

// ProjectionUnit is one synchronous read model driven by every committed
// session event. ApplyProjection must be total: all fallible preparation is
// completed before persistence and the commit boundary. SnapshotProjection
// must return a value detached from the unit's live state.
type ProjectionUnit interface {
	ProjectionKey() string
	ApplyProjection(ProjectionEvent)
	SnapshotProjection() (any, error)
}

// ProjectionSnapshot is one consistent read cut across all registered units.
type ProjectionSnapshot struct {
	AsOfSeq int64
	Values  map[string]any
}

// ProjectionRegistry eagerly drives registered units over committed events.
// It is intentionally lock-free; the owning session journal serializes commit
// and snapshot access.
type ProjectionRegistry struct {
	units   []ProjectionUnit
	keys    map[string]ProjectionUnit
	asOfSeq int64
}

func NewProjectionRegistry() *ProjectionRegistry {
	return &ProjectionRegistry{
		keys:    make(map[string]ProjectionUnit),
		asOfSeq: -1,
	}
}

// Register adds a unit before replay or live events begin.
func (r *ProjectionRegistry) Register(unit ProjectionUnit) error {
	if r == nil {
		return fmt.Errorf("transcript: projection registry is nil")
	}
	if unit == nil {
		return fmt.Errorf("transcript: projection unit is nil")
	}
	rawKey := unit.ProjectionKey()
	key := strings.TrimSpace(rawKey)
	if key == "" {
		return fmt.Errorf("transcript: projection key is empty")
	}
	if key != rawKey {
		return fmt.Errorf("transcript: projection key %q has surrounding whitespace", rawKey)
	}
	if r.asOfSeq >= 0 {
		return fmt.Errorf("transcript: projection %q registered after replay began", key)
	}
	if _, exists := r.keys[key]; exists {
		return fmt.Errorf("transcript: projection key %q is already registered", key)
	}
	r.keys[key] = unit
	r.units = append(r.units, unit)
	return nil
}

func (r *ProjectionRegistry) apply(events []ProjectionEvent) {
	if r == nil {
		return
	}
	for _, event := range events {
		want := r.asOfSeq + 1
		if event.Entry.Seq != want {
			panic(fmt.Sprintf(
				"transcript: projection event sequence %d, want %d",
				event.Entry.Seq,
				want,
			))
		}
		for _, unit := range r.units {
			unit.ApplyProjection(event)
		}
		r.asOfSeq = event.Entry.Seq
	}
}

// Snapshot returns detached values from the same committed sequence.
func (r *ProjectionRegistry) Snapshot() (ProjectionSnapshot, error) {
	if r == nil {
		return ProjectionSnapshot{}, fmt.Errorf("transcript: projection registry is nil")
	}
	values := make(map[string]any, len(r.units))
	for _, unit := range r.units {
		value, err := unit.SnapshotProjection()
		if err != nil {
			return ProjectionSnapshot{}, fmt.Errorf(
				"transcript: snapshot projection %q: %w",
				unit.ProjectionKey(),
				err,
			)
		}
		values[unit.ProjectionKey()] = value
	}
	return ProjectionSnapshot{AsOfSeq: r.asOfSeq, Values: values}, nil
}

// SnapshotKey returns one detached projection at the registry's committed
// watermark without forcing unrelated units to clone their state.
func (r *ProjectionRegistry) SnapshotKey(key string) (ProjectionSnapshot, error) {
	if r == nil {
		return ProjectionSnapshot{}, fmt.Errorf("transcript: projection registry is nil")
	}
	unit, ok := r.keys[key]
	if !ok {
		return ProjectionSnapshot{}, fmt.Errorf(
			"transcript: projection %q is not registered",
			key,
		)
	}
	value, err := unit.SnapshotProjection()
	if err != nil {
		return ProjectionSnapshot{}, fmt.Errorf(
			"transcript: snapshot projection %q: %w",
			key,
			err,
		)
	}
	return ProjectionSnapshot{
		AsOfSeq: r.asOfSeq,
		Values:  map[string]any{key: value},
	}, nil
}

func newProjectionEvent(
	index int,
	entry Entry,
	transition sessionTransition,
) (ProjectionEvent, error) {
	event := ProjectionEvent{
		EntryIndex: index,
		Scope: ProjectedLifecycle{
			RunID: transition.Scope.RunID, TurnID: transition.Scope.TurnID,
			StepID: transition.Scope.StepID,
		},
	}
	if transition.Message != nil {
		message, err := cloneProjectedMessage(transition.Message)
		if err != nil {
			return ProjectionEvent{}, fmt.Errorf(
				"transcript: prepare projection for message entry %s: %w",
				entry.ID,
				err,
			)
		}
		event.message = message
	}
	event.Entry = cloneProjectionEntry(entry, event.message)
	for _, request := range transition.ToolRequests {
		event.toolRequests = append(event.toolRequests, projectedToolRequest{
			ID: request.Request.ID, Name: request.Request.Name,
			Arguments: append([]byte(nil), request.Request.Arguments...),
			Scope: ProjectedLifecycle{
				RunID: request.Scope.RunID, TurnID: request.Scope.TurnID,
				StepID: request.Scope.StepID,
			},
			AssistantEntryID:    request.AssistantEntryID,
			AssistantEntryIndex: request.AssistantEntryIndex,
		})
	}
	if transition.Tool != nil && transition.Message != nil {
		event.toolResultID = transition.Tool.Request.ID
	}
	return event, nil
}

func cloneProjectionEntry(entry Entry, message agent.AgentMessage) Entry {
	clone := entry
	if message != nil {
		clone.Message = message
	}
	if entry.ToolCall != nil {
		call := *entry.ToolCall
		call.Arguments = append([]byte(nil), entry.ToolCall.Arguments...)
		clone.ToolCall = &call
	}
	if entry.ToolOutcome != nil {
		outcome := cloneToolOutcome(*entry.ToolOutcome)
		clone.ToolOutcome = &outcome
	}
	if entry.Context != nil {
		context := *entry.Context
		clone.Context = &context
	}
	if entry.Compaction != nil {
		compaction := *entry.Compaction
		compaction.ReadFiles = append([]string(nil), entry.Compaction.ReadFiles...)
		compaction.ModifiedFiles = append([]string(nil), entry.Compaction.ModifiedFiles...)
		clone.Compaction = &compaction
	}
	if entry.Lifecycle != nil {
		lifecycle := *entry.Lifecycle
		clone.Lifecycle = &lifecycle
	}
	if entry.RequestHeader != nil {
		header := cloneRequestHeader(*entry.RequestHeader)
		clone.RequestHeader = &header
	}
	return clone
}

func cloneToolOutcome(source ToolOutcome) ToolOutcome {
	clone := source
	clone.Data = append([]byte(nil), source.Data...)
	if source.ExitCode != nil {
		exitCode := *source.ExitCode
		clone.ExitCode = &exitCode
	}
	return clone
}
