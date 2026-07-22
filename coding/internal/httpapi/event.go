// Package httpapi is the product's HTTP delivery layer. It streams run events over
// Server-Sent Events and accepts prompts and permission answers over POST
// requests.
//
// Everything it serves lives below the application composition root, so another
// client can drive the same conversations without owning product construction.
package httpapi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/tools"
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
	Final  bool        `json:"finalResponse,omitempty"`
	// Completed-response metadata. ModelName is the stable catalog display name;
	// Provider and Model keep the exact identity available to other clients.
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	ModelName string `json:"modelName,omitempty"`
	// queued-message metadata
	Delivery string `json:"delivery,omitempty"`
	Queued   bool   `json:"queued,omitempty"`
	// approval_request
	ID      string `json:"id,omitempty"`
	Summary string `json:"summary,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// title_update
	Title       string `json:"title,omitempty"`
	AITitle     string `json:"aiTitle,omitempty"`
	CustomTitle string `json:"customTitle,omitempty"`
	// run timing
	StartedAt  string `json:"startedAt,omitempty"`
	DurationMS *int64 `json:"durationMs,omitempty"`
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

type wireContextUsage struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	UsedTokens    int64  `json:"usedTokens"`
	ContextWindow int64  `json:"contextWindow"`
	Measured      bool   `json:"measured"`
}

func projectContextUsage(usage engine.ContextUsage) wireContextUsage {
	return wireContextUsage{
		Provider:      usage.Provider,
		Model:         usage.Model,
		UsedTokens:    usage.UsedTokens,
		ContextWindow: usage.ContextWindow,
		Measured:      usage.Measured,
	}
}

// ProjectEvent maps a UI-neutral coding event to the HTTP wire protocol.
func ProjectEvent(ev engine.Event) ([]byte, bool) {
	var out wireEvent
	switch ev.Type {
	case engine.RunStarted:
		out = wireEvent{Type: "run_start", StartedAt: formatEventTime(ev.StartedAt)}

	case engine.UserMessageCompleted:
		out = wireEvent{Type: "user_message", Text: ev.Text, Images: projectImages(ev.Images)}

	case engine.TextDelta:
		out = wireEvent{Type: "delta", Kind: "text", Delta: ev.Delta}

	case engine.ThinkingDelta:
		out = wireEvent{Type: "delta", Kind: "thinking", Delta: ev.Delta}

	case engine.ToolStarted:
		out = wireEvent{Type: "tool_start", ID: ev.ToolCallID, Tool: ev.ToolName, Args: ev.ToolArgs}

	case engine.ToolFinished:
		out = wireEvent{Type: "tool_end", ID: ev.ToolCallID, Tool: ev.ToolName, Result: wireToolResult(ev.ToolName, ev.ToolResult), Change: fileChangePayload(ev.ToolDetails), IsError: ev.IsError}

	case engine.MessageCompleted:
		out = wireEvent{
			Type:      "message_end",
			Text:      ev.Text,
			Usage:     projectUsage(ev.Usage),
			Final:     ev.FinalResponse,
			Provider:  ev.Provider,
			Model:     ev.Model,
			ModelName: displayModelName(ev.Provider, ev.Model),
		}

	case engine.TurnDiscarded:
		out = wireEvent{Type: "turn_discard"}

	case engine.CompactionStarted:
		if !ev.Automatic {
			return nil, false
		}
		out = wireEvent{Type: "compaction_start"}

	case engine.CompactionCompleted:
		if !ev.Automatic {
			return nil, false
		}
		out = wireEvent{Type: "compaction_end"}

	case engine.CompactionFailed:
		if !ev.Automatic {
			return nil, false
		}
		out = wireEvent{Type: "compaction_end", IsError: true, Text: ev.Error}

	case engine.RunCompleted:
		out = wireEvent{
			Type:       "done",
			Usage:      projectUsage(ev.Usage),
			StartedAt:  formatEventTime(ev.StartedAt),
			DurationMS: elapsedMilliseconds(ev.StartedAt, ev.CompletedAt),
		}

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
func ProjectHistory(items []engine.HistoryItem) []wireEvent {
	out := make([]wireEvent, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case engine.HistoryRun:
			out = append(out, wireEvent{
				Type:       "run_start",
				StartedAt:  formatEventTime(item.StartedAt),
				DurationMS: elapsedMilliseconds(item.StartedAt, item.CompletedAt),
			})

		case engine.HistoryUser:
			out = append(out, wireEvent{Type: "user_message", Text: item.Text, Images: projectImages(item.Images)})

		case engine.HistoryAssistant:
			out = append(out, wireEvent{
				Type:      "message_end",
				Text:      item.Text,
				Final:     item.FinalResponse,
				Provider:  item.Provider,
				Model:     item.Model,
				ModelName: displayModelName(item.Provider, item.Model),
			})

		case engine.HistoryThinking:
			out = append(out, wireEvent{Type: "delta", Kind: "thinking", Delta: item.Text})

		case engine.HistoryToolCall:
			out = append(out, wireEvent{
				Type: "tool_start",
				ID:   item.ToolCallID,
				Tool: item.ToolName,
				Args: item.ToolArgs,
			})

		case engine.HistoryToolResult:
			out = append(out, wireEvent{
				Type:    "tool_end",
				ID:      item.ToolCallID,
				Tool:    item.ToolName,
				Result:  wireToolResult(item.ToolName, item.ToolResult),
				Change:  fileChangePayload(item.ToolDetails),
				IsError: item.IsError,
			})

		case engine.HistoryUsage:
			for index := len(out) - 1; index >= 0; index-- {
				if out[index].Type == "user_message" {
					break
				}
				if out[index].Type == "message_end" && out[index].Final {
					out[index].Usage = projectUsage(item.Usage)
					break
				}
			}
		}
	}
	return out
}

func formatEventTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func elapsedMilliseconds(startedAt, completedAt time.Time) *int64 {
	if startedAt.IsZero() || completedAt.IsZero() || completedAt.Before(startedAt) {
		return nil
	}
	duration := completedAt.Sub(startedAt).Milliseconds()
	return &duration
}

func displayModelName(provider, modelID string) string {
	if model, ok := llm.LookupModel(provider, modelID); ok && model.Name != "" {
		return model.Name
	}
	return modelID
}

func projectImages(images []llm.ImageContent) []wireImage {
	out := make([]wireImage, 0, len(images))
	for _, image := range images {
		out = append(out, wireImage{Data: image.Data, MIMEType: image.MIMEType})
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
// shape, tagged so the client can tell a successful change from a failure.
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
