package tools

import (
	"fmt"
	"strings"
)

// Default output caps for tool results. Unbounded file reads or command output
// can fill the context window in a single turn, so results are capped and the
// model is told what was dropped.
const (
	DefaultMaxLines = 1000
	DefaultMaxBytes = 30000
)

// truncate caps s to at most maxLines lines and maxBytes bytes, keeping the head
// and appending a one-line notice describing what was removed. A non-positive
// limit disables that dimension. The line cap is applied before the byte cap.
func truncate(s string, maxLines, maxBytes int) string {
	var notice string

	if maxLines > 0 {
		lines := strings.Split(s, "\n")
		if len(lines) > maxLines {
			dropped := len(lines) - maxLines
			s = strings.Join(lines[:maxLines], "\n")
			notice = fmt.Sprintf("\n\n[truncated: %d more line(s) not shown]", dropped)
		}
	}

	if maxBytes > 0 && len(s) > maxBytes {
		s = s[:maxBytes]
		notice = fmt.Sprintf("\n\n[truncated: output exceeded %d bytes]", maxBytes)
	}

	return s + notice
}
