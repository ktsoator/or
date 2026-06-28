package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// validJSONEscapes is the set of characters that may legally follow a backslash
// inside a JSON string.
var validJSONEscapes = map[byte]bool{
	'"': true, '\\': true, '/': true, 'b': true, 'f': true,
	'n': true, 'r': true, 't': true, 'u': true,
}

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
	if arguments, ok := decodeJSONObject(raw); ok {
		return arguments, ArgumentsStrict
	}
	repaired := RepairJSON(raw)
	if repaired != raw {
		if arguments, ok := decodeJSONObject(repaired); ok {
			return arguments, ArgumentsRepaired
		}
	}
	// Streamed arguments may be truncated mid-token; close the open structures
	// and decode the prefix received so far, on the raw then the repaired copy.
	if arguments, ok := parsePartialJSONObject(raw); ok {
		return arguments, ArgumentsPartial
	}
	if repaired != raw {
		if arguments, ok := parsePartialJSONObject(repaired); ok {
			return arguments, ArgumentsPartial
		}
	}
	return map[string]any{}, ArgumentsInvalid
}

func decodeJSONObject(raw string) (map[string]any, bool) {
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil || arguments == nil {
		return nil, false
	}
	return arguments, true
}

// parsePartialJSONObject closes the open containers and strings of a truncated
// JSON document, then decodes the completed text as an object. ok is false when
// the input cannot be completed into a valid JSON object.
func parsePartialJSONObject(raw string) (map[string]any, bool) {
	completed, ok := completeJSON(raw)
	if !ok {
		return nil, false
	}
	return decodeJSONObject(completed)
}

// RepairJSON fixes malformed JSON string literals by escaping raw control
// characters inside strings and doubling backslashes before invalid escape
// characters, while preserving valid escapes and \uXXXX sequences. Only ASCII
// characters carry special meaning here, so iterating over bytes leaves
// multi-byte UTF-8 sequences untouched.
func RepairJSON(source string) string {
	var repaired strings.Builder
	repaired.Grow(len(source))
	inString := false

	for index := 0; index < len(source); index++ {
		char := source[index]

		if !inString {
			repaired.WriteByte(char)
			if char == '"' {
				inString = true
			}
			continue
		}

		if char == '"' {
			repaired.WriteByte(char)
			inString = false
			continue
		}

		if char == '\\' {
			if index+1 >= len(source) {
				repaired.WriteString(`\\`)
				continue
			}
			next := source[index+1]

			if next == 'u' && index+6 <= len(source) && isHex4(source[index+2:index+6]) {
				repaired.WriteString(source[index : index+6])
				index += 5
				continue
			}

			if validJSONEscapes[next] {
				repaired.WriteByte('\\')
				repaired.WriteByte(next)
				index++
				continue
			}

			repaired.WriteString(`\\`)
			continue
		}

		if char <= 0x1f {
			repaired.WriteString(escapeControlByte(char))
		} else {
			repaired.WriteByte(char)
		}
	}

	return repaired.String()
}

func escapeControlByte(char byte) string {
	switch char {
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	default:
		return fmt.Sprintf(`\u%04x`, char)
	}
}

func isHex4(value string) bool {
	if len(value) != 4 {
		return false
	}
	for index := 0; index < 4; index++ {
		char := value[index]
		isHex := (char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}
