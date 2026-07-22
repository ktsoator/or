package engine

import (
	"encoding/json"

	"github.com/ktsoator/or/coding/internal/tools"
)

// This file (de)serializes the structured Details a tool attaches to its result
// so they can be persisted out of band and restored when a session reloads. Only
// the known coding-tool result types are handled; anything else is skipped and
// simply falls back to text on reload.

const (
	kindFileChange      = "file_change"
	kindMutationFailure = "mutation_failure"
)

// detailsEnvelope tags a persisted payload with its concrete type so decode can
// reconstruct the same Go value the live event carried.
type detailsEnvelope struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// encodeDetails serializes a tool's Details into a tagged payload. It reports
// false for values it does not recognize, which callers skip.
func encodeDetails(details any) (json.RawMessage, bool) {
	var kind string
	switch details.(type) {
	case tools.FileChange:
		kind = kindFileChange
	case tools.MutationFailure:
		kind = kindMutationFailure
	default:
		return nil, false
	}
	data, err := json.Marshal(details)
	if err != nil {
		return nil, false
	}
	raw, err := json.Marshal(detailsEnvelope{Kind: kind, Data: data})
	if err != nil {
		return nil, false
	}
	return raw, true
}

// decodeDetails reconstructs a tool's Details from a tagged payload. It returns
// nil for an unrecognized or malformed payload, leaving history to fall back to
// text.
func decodeDetails(raw json.RawMessage) any {
	var env detailsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}
	switch env.Kind {
	case kindFileChange:
		var v tools.FileChange
		if err := json.Unmarshal(env.Data, &v); err != nil {
			return nil
		}
		return v
	case kindMutationFailure:
		var v tools.MutationFailure
		if err := json.Unmarshal(env.Data, &v); err != nil {
			return nil
		}
		return v
	default:
		return nil
	}
}
