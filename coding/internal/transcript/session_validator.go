package transcript

import (
	"fmt"
	"math"
)

// SessionValidator is an immutable validation cursor over one committed event
// prefix. ValidateAppend applies a batch to a private reducer copy, so callers
// can install the returned cursor only after the same batch is durable.
type SessionValidator struct {
	reducer *sessionReducer
	applied int64
}

// ValidateSession replays a complete committed event prefix.
func ValidateSession(entries []Entry) (*SessionValidator, error) {
	validator := &SessionValidator{
		reducer: newSessionReducer(len(entries)),
	}
	return validator.ValidateAppend(entries)
}

// ValidateAppend returns a new cursor advanced through entries. The receiver
// remains unchanged when any entry is invalid.
func (v *SessionValidator) ValidateAppend(entries []Entry) (*SessionValidator, error) {
	if v == nil || v.reducer == nil {
		return nil, fmt.Errorf("transcript: session validator is nil")
	}
	if len(entries) == 0 {
		return v, nil
	}
	if v.applied > int64(math.MaxInt)-int64(len(entries)) {
		return nil, fmt.Errorf("transcript: session sequence exceeds platform limits")
	}

	next := &SessionValidator{
		reducer: v.reducer.clone(),
		applied: v.applied,
	}
	for _, entry := range entries {
		if _, err := next.reducer.Apply(int(next.applied), entry); err != nil {
			return nil, err
		}
		next.applied++
	}
	return next, nil
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
	}
	return sequenced, nil
}
