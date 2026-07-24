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
		out = wireEvent{Type: wireEventRunStart, StartedAt: formatEventTime(ev.StartedAt)}

	case engine.UserMessageCompleted:
		out = wireEvent{Type: wireEventUserMessage, Text: ev.Text, Images: projectImages(ev.Images)}

	case engine.TextDelta:
		out = wireEvent{Type: wireEventDelta, Kind: wireDeltaText, Delta: ev.Delta}

	case engine.ThinkingDelta:
		out = wireEvent{Type: wireEventDelta, Kind: wireDeltaThinking, Delta: ev.Delta}

	case engine.ToolInputStarted:
		out = wireEvent{Type: wireEventToolInputStart, ID: ev.ToolCallID, Tool: ev.ToolName, ToolContentIndex: intPointer(ev.ToolContentIndex)}

	case engine.ToolInputDelta:
		out = wireEvent{Type: wireEventToolInputDelta, ID: ev.ToolCallID, Tool: ev.ToolName, ToolContentIndex: intPointer(ev.ToolContentIndex), Bytes: ev.ToolInputBytes}

	case engine.ToolInputCompleted:
		out = wireEvent{Type: wireEventToolInputEnd, ID: ev.ToolCallID, Tool: ev.ToolName, Args: ev.ToolArgs, ToolContentIndex: intPointer(ev.ToolContentIndex)}

	case engine.ToolStarted:
		out = wireEvent{Type: wireEventToolStart, ID: ev.ToolCallID, Tool: ev.ToolName, Args: ev.ToolArgs}

	case engine.ToolFinished:
		out = wireEvent{Type: wireEventToolEnd, ID: ev.ToolCallID, Tool: ev.ToolName, Result: wireToolResult(ev.ToolName, ev.ToolResult), Change: fileChangePayload(ev.ToolDetails), Preview: previewPayload(ev.ToolDetails), IsError: ev.IsError}

	case engine.MessageCompleted:
		out = wireEvent{
			Type:        wireEventMessageEnd,
			Text:        ev.Text,
			Usage:       projectUsage(ev.Usage),
			Final:       ev.FinalResponse,
			Provider:    ev.Provider,
			Model:       ev.Model,
			ModelName:   displayModelName(ev.Provider, ev.Model),
			CompletedAt: formatEventTime(ev.CompletedAt),
		}

	case engine.TurnDiscarded:
		out = wireEvent{Type: wireEventTurnDiscard}

	case engine.CompactionStarted:
		if !ev.Automatic {
			return nil, false
		}
		out = wireEvent{Type: wireEventCompactionStart}

	case engine.CompactionCompleted:
		if !ev.Automatic {
			return nil, false
		}
		out = wireEvent{Type: wireEventCompactionEnd}

	case engine.CompactionFailed:
		if !ev.Automatic {
			return nil, false
		}
		out = wireEvent{Type: wireEventCompactionEnd, IsError: true, Text: ev.Error}

	case engine.RunCompleted:
		out = wireEvent{
			Type:       wireEventDone,
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

func intPointer(value int) *int {
	return &value
}

// ProjectHistory maps a UI-neutral conversation snapshot to the same event
// shapes the browser already renders for live activity.
func ProjectHistory(items []engine.HistoryItem) []wireEvent {
	out := make([]wireEvent, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case engine.HistoryRun:
			out = append(out, wireEvent{
				Type:       wireEventRunStart,
				StartedAt:  formatEventTime(item.StartedAt),
				DurationMS: elapsedMilliseconds(item.StartedAt, item.CompletedAt),
			})

		case engine.HistoryUser:
			out = append(out, wireEvent{Type: wireEventUserMessage, Text: item.Text, Images: projectImages(item.Images)})

		case engine.HistoryAssistant:
			out = append(out, wireEvent{
				Type:        wireEventMessageEnd,
				Text:        item.Text,
				Final:       item.FinalResponse,
				Provider:    item.Provider,
				Model:       item.Model,
				ModelName:   displayModelName(item.Provider, item.Model),
				CompletedAt: formatEventTime(item.CompletedAt),
			})

		case engine.HistoryThinking:
			out = append(out, wireEvent{Type: wireEventDelta, Kind: wireDeltaThinking, Delta: item.Text})

		case engine.HistoryToolCall:
			out = append(out, wireEvent{
				Type: wireEventToolStart,
				ID:   item.ToolCallID,
				Tool: item.ToolName,
				Args: item.ToolArgs,
			})

		case engine.HistoryToolResult:
			out = append(out, wireEvent{
				Type:    wireEventToolEnd,
				ID:      item.ToolCallID,
				Tool:    item.ToolName,
				Result:  wireToolResult(item.ToolName, item.ToolResult),
				Change:  fileChangePayload(item.ToolDetails),
				Preview: previewPayload(item.ToolDetails),
				IsError: item.IsError,
			})

		case engine.HistoryUsage:
			for index := len(out) - 1; index >= 0; index-- {
				if out[index].Type == wireEventUserMessage {
					break
				}
				if out[index].Type == wireEventMessageEnd && out[index].Final {
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
func fileChangePayload(details any) wireChange {
	switch d := details.(type) {
	case tools.FileChange:
		hunks := make([]wireHunk, len(d.Hunks))
		for i, h := range d.Hunks {
			hunks[i] = wireHunk{
				OldStart: h.OldStart,
				OldLines: h.OldLines,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
				Lines:    h.Lines,
			}
		}
		return wireFileChangePayload{
			ChangeType: wireChangeFile,
			Path:       d.Path,
			Operation:  projectFileOperation(d.Kind),
			Additions:  d.Additions,
			Deletions:  d.Deletions,
			Bytes:      d.Bytes,
			Hunks:      hunks,
		}
	case tools.MutationFailure:
		return wireFailureChangePayload{
			ChangeType: wireChangeFailure,
			Path:       d.Path,
			Reason:     d.Reason,
			Detail:     d.Detail,
		}
	default:
		return nil
	}
}

func projectFileOperation(kind tools.ChangeKind) wireFileOperation {
	switch kind {
	case tools.ChangeCreate:
		return wireFileCreate
	case tools.ChangeUpdate:
		return wireFileUpdate
	default:
		return wireFileOperation(kind)
	}
}

func previewPayload(details any) *wirePreview {
	preview, ok := details.(tools.PreviewRequest)
	if !ok {
		return nil
	}
	return &wirePreview{
		URL:          preview.URL,
		Path:         preview.Path,
		RelativePath: preview.RelativePath,
		Title:        preview.Title,
		GrantID:      preview.GrantID,
		PreviewPath:  preview.PreviewPath,
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
