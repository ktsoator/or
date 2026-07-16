// Package web is a browser front-end for a coding session. It is part of the
// product shell: it consumes the same coding.Session the terminal shell uses,
// streaming run events to the browser over Server-Sent Events and taking prompts
// and permission answers back over plain POST requests. The coding core has no
// knowledge of it.
package web

import (
	"encoding/json"
	"strings"

	"github.com/ktsoator/or/coding"
)

// wireEvent is the JSON shape streamed to the browser. Fields are populated
// according to Type; the rest stay zero and are omitted.
type wireEvent struct {
	Type string `json:"type"`
	// delta events
	Kind  string `json:"kind,omitempty"`  // "text" or "thinking"
	Delta string `json:"delta,omitempty"` // incremental content
	// tool events (ID correlates tool_start with tool_end)
	Tool    string `json:"tool,omitempty"`
	Args    any    `json:"args,omitempty"`
	Result  string `json:"result,omitempty"`
	IsError bool   `json:"isError,omitempty"`
	// message_end fallback text (used when nothing streamed)
	Text string `json:"text,omitempty"`
	// confirm_request
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// ProjectEvent maps a UI-neutral coding event to the Web wire protocol.
func ProjectEvent(ev coding.Event) ([]byte, bool) {
	var out wireEvent
	switch ev.Type {
	case coding.TextDelta:
		out = wireEvent{Type: "delta", Kind: "text", Delta: ev.Delta}

	case coding.ThinkingDelta:
		out = wireEvent{Type: "delta", Kind: "thinking", Delta: ev.Delta}

	case coding.ToolStarted:
		out = wireEvent{Type: "tool_start", ID: ev.ToolCallID, Tool: ev.ToolName, Args: ev.ToolArgs}

	case coding.ToolFinished:
		out = wireEvent{Type: "tool_end", ID: ev.ToolCallID, Tool: ev.ToolName, Result: firstLines(ev.ToolResult, 12), IsError: ev.IsError}

	case coding.MessageCompleted:
		out = wireEvent{Type: "message_end", Text: ev.Text}

	case coding.RunCompleted:
		out = wireEvent{Type: "done"}

	default:
		return nil, false
	}

	data, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return data, true
}

// firstLines returns at most n lines of s.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n")
}
