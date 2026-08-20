package transcript

import (
	"fmt"
	"math"
)

// SessionValidator owns the canonical reducer for one committed event prefix.
// PrepareAppend validates against a batch-local delta that callers commit only
// after the same entries are durable.
type SessionValidator struct {
	reducer     *sessionReducer
	projections *ProjectionRegistry
	applied     int64
}

// PreparedAppend is a validated batch-local reducer delta. Commit installs it
// into its originating SessionValidator and must be called at most once.
type PreparedAppend struct {
	validator *SessionValidator
	stage     *sessionReducer
	firstSeq  int64
	count     int64
	committed bool
	events    []ProjectionEvent
}

// ValidateSession replays a complete committed event prefix.
func ValidateSession(entries []Entry) (*SessionValidator, error) {
	return validateSession(entries, nil)
}

func validateSession(
	entries []Entry,
	projections *ProjectionRegistry,
) (*SessionValidator, error) {
	validator := &SessionValidator{
		reducer:     newSessionReducer(len(entries)),
		projections: projections,
	}
	for index, entry := range entries {
		transition, err := validator.reducer.Apply(index, entry)
		if err != nil {
			return nil, err
		}
		if projections != nil {
			event, err := newProjectionEvent(index, entry, transition)
			if err != nil {
				return nil, err
			}
			projections.apply([]ProjectionEvent{event})
		}
		validator.applied++
	}
	return validator, nil
}

// PrepareAppend validates entries without changing the committed cursor.
func (v *SessionValidator) PrepareAppend(entries []Entry) (*PreparedAppend, error) {
	if v == nil || v.reducer == nil {
		return nil, fmt.Errorf("transcript: session validator is nil")
	}
	if v.applied > int64(math.MaxInt)-int64(len(entries)) {
		return nil, fmt.Errorf("transcript: session sequence exceeds platform limits")
	}

	stage := newStagedSessionReducer(v.reducer, len(entries))
	applied := v.applied
	var events []ProjectionEvent
	if v.projections != nil {
		events = make([]ProjectionEvent, 0, len(entries))
	}
	for _, entry := range entries {
		transition, err := stage.Apply(int(applied), entry)
		if err != nil {
			return nil, err
		}
		if events != nil {
			event, err := newProjectionEvent(int(applied), entry, transition)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}
		applied++
	}
	return &PreparedAppend{
		validator: v,
		stage:     stage,
		firstSeq:  v.applied,
		count:     int64(len(entries)),
		events:    events,
	}, nil
}

// Commit installs a prepared delta. A stale or repeated commit is a caller
// programming error; journal serialization keeps production commits ordered.
func (p *PreparedAppend) Commit() {
	if p == nil || p.validator == nil || p.stage == nil || p.committed {
		panic("transcript: invalid prepared append commit")
	}
	if p.validator.applied != p.firstSeq {
		panic("transcript: prepared append is stale")
	}
	p.validator.reducer.commitStage(p.stage)
	p.validator.projections.apply(p.events)
	p.validator.applied += p.count
	p.committed = true
}

// NextSeq is the sequence required for the next committed entry.
func (v *SessionValidator) NextSeq() int64 {
	if v == nil {
		return 0
	}
	return v.applied
}

// SequenceEntries returns a detached batch numbered from firstSeq.
func SequenceEntries(entries []Entry, firstSeq int64) ([]Entry, error) {
	if firstSeq < 0 {
		return nil, fmt.Errorf("transcript: first sequence must be non-negative, got %d", firstSeq)
	}
	if len(entries) > 0 && int64(len(entries)-1) > math.MaxInt64-firstSeq {
		return nil, fmt.Errorf("transcript: event batch exceeds sequence range")
	}
	sequenced := append([]Entry(nil), entries...)
	for index := range sequenced {
		sequenced[index].Seq = firstSeq + int64(index)
		if sequenced[index].RequestHeader != nil {
			header := cloneRequestHeader(*sequenced[index].RequestHeader)
			header.InputSeq = sequenced[index].Seq - 1
			sequenced[index].RequestHeader = &header
		}
	}
	return sequenced, nil
}
