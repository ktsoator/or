// Package web exposes an HTTP API for a coding session. It consumes the same
// coding.Session the terminal shell uses, streams run events over Server-Sent
// Events, and accepts prompts and permission answers over POST requests. The
// coding core and independently deployed React application do not depend on one
// another.
package web

import (
	"encoding/json"
	"strings"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/tools"
	"github.com/ktsoator/or/llm"
)

// wireEvent is the JSON shape streamed to the browser. Fields are populated
// according to Type; the rest stay zero and are omitted.
type wireEvent struct {
	Type string `json:"type"`
	// delta events
	Kind  string `json:"kind,omitempty"`  // "text" or "thinking"
	Delta string `json:"delta,omitempty"` // incremental content
	// tool events (ID correlates tool_start with tool_end)
	Tool   string `json:"tool,omitempty"`
	Args   any    `json:"args,omitempty"`
	Result string `json:"result,omitempty"`
	// Change is the structured file-change result (tools.FileChange) or failure
	// (tools.MutationFailure), when the tool produced one, for rich rendering.
	Change  any  `json:"change,omitempty"`
	IsError bool `json:"isError,omitempty"`
	// message_end fallback text (used when nothing streamed)
	Text   string      `json:"text,omitempty"`
	Images []wireImage `json:"images,omitempty"`
	Usage  *wireUsage  `json:"usage,omitempty"`
	// confirm_request
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type wireImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type wireUsage struct {
	Input       int64         `json:"input"`
	Output      int64         `json:"output"`
	CacheRead   int64         `json:"cacheRead"`
	CacheWrite  int64         `json:"cacheWrite"`
	TotalTokens int64         `json:"totalTokens"`
	Cost        wireUsageCost `json:"cost"`
}

type wireUsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// ProjectEvent maps a UI-neutral coding event to the HTTP wire protocol.
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
		out = wireEvent{Type: "tool_end", ID: ev.ToolCallID, Tool: ev.ToolName, Result: wireToolResult(ev.ToolName, ev.ToolResult), Change: fileChangePayload(ev.ToolDetails), IsError: ev.IsError}

	case coding.MessageCompleted:
		out = wireEvent{Type: "message_end", Text: ev.Text}

	case coding.RunCompleted:
		out = wireEvent{Type: "done", Usage: projectUsage(ev.Usage)}

	default:
		return nil, false
	}

	data, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return data, true
}

// ProjectHistory maps a UI-neutral conversation snapshot to the same event
// shapes the browser already renders for live activity.
func ProjectHistory(items []coding.HistoryItem) []wireEvent {
	out := make([]wireEvent, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case coding.HistoryUser:
			images := make([]wireImage, 0, len(item.Images))
			for _, image := range item.Images {
				images = append(images, wireImage{Data: image.Data, MIMEType: image.MIMEType})
			}
			out = append(out, wireEvent{Type: "user_message", Text: item.Text, Images: images})

		case coding.HistoryAssistant:
			out = append(out, wireEvent{Type: "message_end", Text: item.Text})

		case coding.HistoryThinking:
			out = append(out, wireEvent{Type: "delta", Kind: "thinking", Delta: item.Text})

		case coding.HistoryToolCall:
			out = append(out, wireEvent{
				Type: "tool_start",
				ID:   item.ToolCallID,
				Tool: item.ToolName,
				Args: item.ToolArgs,
			})

		case coding.HistoryToolResult:
			out = append(out, wireEvent{
				Type:    "tool_end",
				ID:      item.ToolCallID,
				Tool:    item.ToolName,
				Result:  wireToolResult(item.ToolName, item.ToolResult),
				Change:  fileChangePayload(item.ToolDetails),
				IsError: item.IsError,
			})

		case coding.HistoryUsage:
			out = append(out, wireEvent{Type: "done", Usage: projectUsage(item.Usage)})
		}
	}
	return out
}

func projectUsage(usage llm.Usage) *wireUsage {
	if usage.Input == 0 && usage.Output == 0 && usage.CacheRead == 0 &&
		usage.CacheWrite == 0 && usage.TotalTokens == 0 && usage.Cost.Total == 0 {
		return nil
	}
	return &wireUsage{
		Input:       usage.Input,
		Output:      usage.Output,
		CacheRead:   usage.CacheRead,
		CacheWrite:  usage.CacheWrite,
		TotalTokens: usage.TotalTokens,
		Cost: wireUsageCost{
			Input:      usage.Cost.Input,
			Output:     usage.Cost.Output,
			CacheRead:  usage.Cost.CacheRead,
			CacheWrite: usage.Cost.CacheWrite,
			Total:      usage.Cost.Total,
		},
	}
}

// wireToolResult keeps file reads inspectable in the browser while retaining a
// compact preview for commands and other tools. The read tool already enforces
// its own line and byte limits.
func wireToolResult(tool, result string) string {
	name := strings.ToLower(tool)
	if strings.Contains(name, "read") || strings.Contains(name, "cat") {
		return result
	}
	return firstLines(result, 12)
}

// fileChangePayload converts a tool's structured Details into the browser wire
// shape, tagged so the front-end can tell a successful change from a failure.
// It returns nil for tools that produced no structured result.
func fileChangePayload(details any) any {
	switch d := details.(type) {
	case tools.FileChange:
		hunks := make([]map[string]any, len(d.Hunks))
		for i, h := range d.Hunks {
			hunks[i] = map[string]any{
				"oldStart": h.OldStart,
				"oldLines": h.OldLines,
				"newStart": h.NewStart,
				"newLines": h.NewLines,
				"lines":    h.Lines,
			}
		}
		return map[string]any{
			"changeType": "file",
			"path":       d.Path,
			"op":         string(d.Kind),
			"additions":  d.Additions,
			"deletions":  d.Deletions,
			"bytes":      d.Bytes,
			"hunks":      hunks,
		}
	case tools.MutationFailure:
		return map[string]any{
			"changeType": "failure",
			"path":       d.Path,
			"reason":     d.Reason,
			"detail":     d.Detail,
		}
	default:
		return nil
	}
}

// firstLines returns at most n lines of s.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n")
}
