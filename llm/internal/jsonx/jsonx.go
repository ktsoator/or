// Package jsonx provides best-effort JSON recovery for model output. Streamed
// tool arguments are often malformed — unescaped control characters, bad
// escapes, or a stream cut off mid-token — so a strict decode would discard
// everything. These helpers repair and complete such input so callers can
// salvage whatever was received instead of failing the surrounding response.
package jsonx

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

// DecodeObject strictly decodes raw into a JSON object. ok is false when the
// input is not a valid, non-null JSON object.
func DecodeObject(raw string) (map[string]any, bool) {
	var arguments map[string]any
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil || arguments == nil {
		return nil, false
	}
	return arguments, true
}

// ParsePartialObject closes the open containers and strings of a truncated JSON
// document, then decodes the completed text as an object. ok is false when the
// input cannot be completed into a valid JSON object.
func ParsePartialObject(raw string) (map[string]any, bool) {
	completed, ok := Complete(raw)
	if !ok {
		return nil, false
	}
	return DecodeObject(completed)
}

// Repair fixes malformed JSON string literals by escaping raw control
// characters inside strings and doubling backslashes before invalid escape
// characters, while preserving valid escapes and \uXXXX sequences. Only ASCII
// characters carry special meaning here, so iterating over bytes leaves
// multi-byte UTF-8 sequences untouched.
func Repair(source string) string {
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
