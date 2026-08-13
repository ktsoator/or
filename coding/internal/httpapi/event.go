// Package httpapi is the product's HTTP delivery layer. It streams run events over
// Server-Sent Events and accepts prompts and permission answers over POST
// requests.
//
// Everything it serves lives below the application composition root, so another
// client can drive the same conversations without owning product construction.
package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func projectContextUsage(usage engine.ContextUsage) wireContextUsage {
	projected := wireContextUsage{
		Provider:      usage.Provider,
		Model:         usage.Model,
		UsedTokens:    usage.UsedTokens,
		ContextWindow: usage.ContextWindow,
		Measured:      usage.Measured,
	}
	if usage.Breakdown != nil {
		projected.Breakdown = &wireContextBreakdown{
			Messages:       usage.Breakdown.Messages,
			SystemTools:    usage.Breakdown.SystemTools,
			SystemPrompt:   usage.Breakdown.SystemPrompt,
			Skills:         usage.Breakdown.Skills,
			ProjectContext: usage.Breakdown.ProjectContext,
		}
	}
	return projected
}

func projectEventContextUsage(usage engine.ContextUsage) *wireContextUsage {
	if usage.Provider == "" && usage.Model == "" && usage.ContextWindow == 0 {
		return nil
	}
	projected := projectContextUsage(usage)
	return &projected
}

// ProjectEvent maps a UI-neutral coding event to the HTTP wire protocol.
func ProjectEvent(ev engine.Event) ([]byte, bool) {
	out, ok := projectEvent(ev)
	if !ok {
		return nil, false
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return data, true
}

func projectEvent(ev engine.Event) (wireEvent, bool) {
	var out wireEvent
	switch ev.Type {
	case engine.RunStarted:
		out = wireEvent{Type: wireEventRunStart, ID: ev.RunID, RunID: ev.RunID, StartedAt: formatEventTime(ev.StartedAt)}

	case engine.UserMessageCompleted:
		out = wireEvent{
			Type:   wireEventUserMessage,
			Text:   ev.Text,
			SentAt: formatEventTime(ev.SentAt),
			Images: projectImages(ev.Images),
			Files:  projectFiles(ev.Files),
		}

	case engine.TextDelta:
		out = wireEvent{Type: wireEventDelta, Kind: wireDeltaText, Delta: ev.Delta}

	case engine.ThinkingDelta:
		out = wireEvent{Type: wireEventDelta, Kind: wireDeltaThinking, Delta: ev.Delta}

	case engine.ToolInputStarted:
		out = wireEvent{Type: wireEventToolInputStart, ID: ev.ToolCallID, Tool: ev.ToolName, ToolContentIndex: intPointer(ev.ToolContentIndex)}

	case engine.ToolInputDelta:
		out = wireEvent{Type: wireEventToolInputDelta, ID: ev.ToolCallID, Tool: ev.ToolName, Delta: ev.Delta, ToolContentIndex: intPointer(ev.ToolContentIndex), Bytes: ev.ToolInputBytes}

	case engine.ToolInputCompleted:
		out = wireEvent{Type: wireEventToolInputEnd, ID: ev.ToolCallID, Tool: ev.ToolName, Args: ev.ToolArgs, ToolContentIndex: intPointer(ev.ToolContentIndex)}

	case engine.ToolStarted:
		out = wireEvent{Type: wireEventToolStart, ID: ev.ToolCallID, Tool: ev.ToolName, Args: ev.ToolArgs}

	case engine.ToolFinished:
		out = wireEvent{Type: wireEventToolEnd, ID: ev.ToolCallID, Tool: ev.ToolName, Result: wireToolResult(ev.ToolResult), Images: projectImages(ev.Images), Outcome: projectToolOutcome(ev.ToolOutcome)}

	case engine.MessageCompleted:
		out = wireEvent{
			Type:        wireEventMessageEnd,
			Text:        ev.Text,
			Usage:       projectUsage(ev.Usage),
			Context:     projectEventContextUsage(ev.ContextUsage),
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
			return wireEvent{}, false
		}
		out = wireEvent{Type: wireEventCompactionStart}

	case engine.CompactionCompleted:
		if !ev.Automatic {
			return wireEvent{}, false
		}
		out = wireEvent{Type: wireEventCompactionEnd}

	case engine.CompactionFailed:
		if !ev.Automatic {
			return wireEvent{}, false
		}
		out = wireEvent{Type: wireEventCompactionEnd, IsError: true, Text: ev.Error}

	case engine.TaskStarted, engine.TaskCompleted:
		eventType := wireEventTaskNotification
		if ev.Type == engine.TaskStarted {
			eventType = wireEventTaskStarted
		}
		out = wireEvent{
			Type: eventType,
			Task: projectBackgroundTask(ev.BackgroundTask),
		}

	case engine.RunCompleted:
		out = wireEvent{
			Type:               wireEventDone,
			RunID:              ev.RunID,
			Usage:              projectUsage(ev.Usage),
			StartedAt:          formatEventTime(ev.StartedAt),
			DurationMS:         elapsedMilliseconds(ev.StartedAt, ev.CompletedAt),
			UserMessageIDs:     ev.UserMessageIDs,
			AssistantMessageID: ev.AssistantMessageID,
		}

	default:
		return wireEvent{}, false
	}
	return out, true
}

func projectBackgroundTask(task engine.BackgroundTask) *wireBackgroundTask {
	return &wireBackgroundTask{
		ID:          task.ID,
		Command:     task.Command,
		Description: task.Description,
		Status:      wireTaskStatus(task.Status),
		OutputPath:  task.OutputPath,
		ExitCode:    task.ExitCode,
		StartedAt:   formatEventTime(task.StartedAt),
		CompletedAt: formatEventTime(task.CompletedAt),
	}
}

func projectBackgroundTasks(tasks []engine.BackgroundTask) []wireBackgroundTask {
	projected := make([]wireBackgroundTask, 0, len(tasks))
	for _, task := range tasks {
		projected = append(projected, *projectBackgroundTask(task))
	}
	return projected
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
				ID:         item.RunID,
				RunID:      item.RunID,
				StartedAt:  formatEventTime(item.StartedAt),
				DurationMS: elapsedMilliseconds(item.StartedAt, item.CompletedAt),
			})

		case engine.HistoryUser:
			out = append(out, wireEvent{
				Type:      wireEventUserMessage,
				Text:      item.Text,
				MessageID: item.MessageID,
				SentAt:    formatEventTime(item.SentAt),
				Images:    projectImages(item.Images),
				Files:     projectFiles(item.Files),
			})

		case engine.HistoryAssistant:
			out = append(out, wireEvent{
				Type:        wireEventMessageEnd,
				Text:        item.Text,
				MessageID:   item.MessageID,
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
				Result:  wireToolResult(item.ToolResult),
				Images:  projectImages(item.Images),
				Outcome: projectToolOutcome(item.ToolOutcome),
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

func projectFiles(files []engine.File) []wireFile {
	out := make([]wireFile, 0, len(files))
	for _, file := range files {
		out = append(out, wireFile{
			Name:     file.Name,
			MIMEType: file.MIMEType,
			Size:     file.Size,
		})
	}
	return out
}

func projectUsage(usage llm.Usage) *wireUsage {
	if !usage.InputUnknown && usage.Input == 0 && usage.Output == 0 && usage.CacheRead == 0 &&
		usage.CacheWrite == 0 && usage.TotalTokens == 0 && usage.Cost.Total == 0 {
		return nil
	}
	return &wireUsage{
		Input:        usage.Input,
		InputUnknown: usage.InputUnknown,
		Output:       usage.Output,
		CacheRead:    usage.CacheRead,
		CacheWrite:   usage.CacheWrite,
		TotalTokens:  usage.TotalTokens,
		Cost: wireUsageCost{
			Input:      usage.Cost.Input,
			Output:     usage.Cost.Output,
			CacheRead:  usage.Cost.CacheRead,
			CacheWrite: usage.Cost.CacheWrite,
			Total:      usage.Cost.Total,
		},
	}
}

const (
	wireToolResultMaxLines = 1000
	wireToolResultMaxBytes = 64 * 1024
)

// wireToolResult preserves inspectable output while keeping SSE and history
// responses bounded. Presentation limits belong to the client; this limit is
// only a transport safeguard and always reports when content was removed.
func wireToolResult(result string) string {
	if len(result) > wireToolResultMaxBytes {
		return limitWireToolResultBytes(result)
	}

	lines := strings.Split(result, "\n")
	if len(lines) > wireToolResultMaxLines {
		dropped := len(lines) - wireToolResultMaxLines
		result = strings.Join(lines[:wireToolResultMaxLines], "\n") +
			fmt.Sprintf("\n\n[truncated: %d more line(s) not shown]", dropped)
	}
	if len(result) <= wireToolResultMaxBytes {
		return result
	}
	return limitWireToolResultBytes(result)
}

func limitWireToolResultBytes(result string) string {
	notice := fmt.Sprintf("\n\n[truncated: tool result exceeded %d bytes]", wireToolResultMaxBytes)
	prefix := result[:wireToolResultMaxBytes-len(notice)]
	for !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix + notice
}

func projectToolOutcome(outcome agent.ToolOutcome) *wireToolOutcome {
	status := wireToolOutcomeStatus(outcome.Status)
	if status == "" {
		status = wireToolOutcomeSuccess
	}
	return &wireToolOutcome{
		Status:    status,
		ErrorCode: outcome.ErrorCode,
		ExitCode:  outcome.ExitCode,
		Data:      projectToolData(outcome.Data),
	}
}

func projectToolData(data any) any {
	switch value := data.(type) {
	case tools.FileChange, tools.MutationFailure:
		return fileChangePayload(value)
	case tools.PreviewRequest:
		return previewPayload(value)
	case tools.QuestionAnswers:
		answers := make([]wireQuestionAnswer, 0, len(value.Answers))
		for _, answer := range value.Answers {
			answers = append(answers, wireQuestionAnswer{Question: answer.Question, Values: answer.Values})
		}
		return wireQuestionAnswers{Questions: projectQuestions(value.Questions), Answers: answers}
	default:
		return data
	}
}

// fileChangePayload converts structured ToolOutcome.Data into the browser wire
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
