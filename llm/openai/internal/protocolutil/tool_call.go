package protocolutil

import (
	"encoding/json"
	"strings"
)

// SanitizeToolCallID replaces characters outside [a-zA-Z0-9_-] with an
// underscore so IDs can be replayed across OpenAI-compatible protocols.
func SanitizeToolCallID(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, id)
}

// TruncateASCII caps an ASCII identifier at limit bytes.
func TruncateASCII(id string, limit int) string {
	if len(id) > limit {
		return id[:limit]
	}
	return id
}

// EncodeToolArguments serializes provider-independent tool arguments for both
// OpenAI wire protocols.
func EncodeToolArguments(arguments map[string]any) (string, error) {
	if arguments == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
