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
// object. Models occasionally emit JSON with unescaped control characters or
// bad escape sequences; a strict decode of that input loses every argument. So
// this first tries a strict decode, then retries on a repaired copy, and only
// then reports the parse error. Empty input represents a tool call with no
// arguments and returns an empty object.
func ParseToolArguments(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	arguments, strictErr := decodeJSONObject(raw)
	if strictErr == nil {
		return arguments, nil
	}
	if repaired := RepairJSON(raw); repaired != raw {
		arguments, repairedErr := decodeJSONObject(repaired)
		if repairedErr == nil {
			return arguments, nil
		}
		return nil, fmt.Errorf("parse repaired tool arguments: %w", repairedErr)
	}
	return nil, fmt.Errorf("parse tool arguments: %w", strictErr)
}

func decodeJSONObject(raw string) (map[string]any, error) {
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return nil, err
	}
	if arguments == nil {
		return nil, fmt.Errorf("tool arguments must be a JSON object")
	}
	return arguments, nil
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
