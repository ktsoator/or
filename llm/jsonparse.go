package llm

import (
	"strings"

	"github.com/ktsoator/or/llm/internal/jsonx"
)

// ParseToolArguments decodes a tool call's accumulated JSON arguments into an
// object on a best-effort basis. Models occasionally emit JSON with unescaped
// control characters or bad escapes, and a stream may be cut off mid-token, so a
// strict decode of that input would lose every argument. This therefore tries,
// in order: a strict decode, a decode of a repaired copy, a partial decode that
// closes any open containers and strings, and a partial decode of the repaired
// copy. It always returns a non-nil map, falling back to an empty object when
// nothing can be salvaged.
//
// Parsing never fails the surrounding stream: a recoverable but invalid tool
// call surfaces with whatever arguments could be salvaged so an agent can still
// validate it (see ValidateToolArguments) and let the model self-correct,
// instead of aborting the whole response.
func ParseToolArguments(raw string) map[string]any {
	arguments, _ := ParseToolArgumentsMode(raw)
	return arguments
}

// ArgumentsMode reports which layer of ParseToolArgumentsMode produced a result,
// so callers can tell strictly parsed arguments apart from recovered ones.
type ArgumentsMode string

const (
	// ArgumentsStrict means empty input or a clean strict decode: fully trusted.
	ArgumentsStrict ArgumentsMode = "strict"
	// ArgumentsRepaired means the input decoded only after escape repair.
	ArgumentsRepaired ArgumentsMode = "repaired"
	// ArgumentsPartial means truncated input was closed and decoded, so some
	// fields may be missing or cut short.
	ArgumentsPartial ArgumentsMode = "partial"
	// ArgumentsInvalid means nothing could be salvaged and the result is empty.
	ArgumentsInvalid ArgumentsMode = "invalid"
)

// ParseToolArgumentsMode is ParseToolArguments with the recovery mode it used.
// The arguments are identical to ParseToolArguments; the mode lets a caller
// decline to execute a tool whose arguments were not strictly parsed.
func ParseToolArgumentsMode(raw string) (map[string]any, ArgumentsMode) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, ArgumentsStrict
	}
	if arguments, ok := jsonx.DecodeObject(raw); ok {
		return arguments, ArgumentsStrict
	}
	repaired := jsonx.Repair(raw)
	if repaired != raw {
		if arguments, ok := jsonx.DecodeObject(repaired); ok {
			return arguments, ArgumentsRepaired
		}
	}
	// Streamed arguments may be truncated mid-token; close the open structures
	// and decode the prefix received so far, on the raw then the repaired copy.
	if arguments, ok := jsonx.ParsePartialObject(raw); ok {
		return arguments, ArgumentsPartial
	}
	if repaired != raw {
		if arguments, ok := jsonx.ParsePartialObject(repaired); ok {
			return arguments, ArgumentsPartial
		}
	}
	return map[string]any{}, ArgumentsInvalid
}
