package trace

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/snapshot"
)

func requestFromGroup(group *requestGroup) Request {
	terminal := group.lifecycle.terminal
	current := terminal
	if current == nil {
		current = group.lifecycle.started
	}
	request := Request{
		ID: group.id, TurnID: group.turnID, StepID: group.stepID,
		Lifecycle: lifecycleState(group.lifecycle),
		StartedAt: lifecycleStart(group.lifecycle), CompletedAt: lifecycleCompleted(group.lifecycle),
		Attempts: []Attempt{}, Checkpoints: []Checkpoint{}, Tools: []Tool{},
		SnapshotState: SnapshotMissing,
		RawEvents:     append([]observability.DiagnosticEvent(nil), group.events...),
	}
	if current != nil {
		request.Status = current.Status
		request.ErrorCode = current.ErrorCode
		request.Provider = current.Provider
		request.Model = current.Model
	}
	if terminal != nil {
		request.DurationMS = terminal.DurationMS
		request.TimeToFirstOutputMS = terminal.TimeToFirstOutputMS
		request.InputTokens = terminal.InputTokens
		request.InputUnknown = terminal.InputUnknown
		request.OutputTokens = terminal.OutputTokens
		request.CacheReadTokens = terminal.CacheReadTokens
		request.CacheWriteTokens = terminal.CacheWriteTokens
		request.TotalTokens = terminal.TotalTokens
		request.CostTotalUSD = terminal.CostTotalUSD
	}
	for _, attempt := range group.attempts {
		request.Attempts = append(
			request.Attempts,
			attemptFromGroup(attempt.id, attempt.number, attempt.lifecycle),
		)
	}
	sort.Slice(request.Attempts, func(i, j int) bool {
		if request.Attempts[i].Number == request.Attempts[j].Number {
			return request.Attempts[i].ID < request.Attempts[j].ID
		}
		return request.Attempts[i].Number < request.Attempts[j].Number
	})
	for _, event := range group.checkpoints {
		checkpoint := Checkpoint{
			Status: event.Status, StartedAt: subtractDuration(event.Timestamp, event.DurationMS),
			CompletedAt: event.Timestamp, DurationMS: event.DurationMS, ErrorCode: event.ErrorCode,
		}
		request.Checkpoints = append(request.Checkpoints, checkpoint)
		request.CheckpointDurationMS += event.DurationMS
	}
	for _, group := range group.tools {
		tool := toolFromGroup(*group)
		request.Tools = append(request.Tools, tool)
	}
	sort.SliceStable(request.Tools, func(i, j int) bool {
		return request.Tools[i].StartedAt.Before(request.Tools[j].StartedAt)
	})
	sort.SliceStable(request.RawEvents, func(i, j int) bool {
		return request.RawEvents[i].Timestamp.Before(request.RawEvents[j].Timestamp)
	})
	return request
}

func attemptFromGroup(id string, number int, group lifecycleGroup) Attempt {
	current := group.terminal
	if current == nil {
		current = group.started
	}
	attempt := Attempt{
		ID: id, Number: number, Lifecycle: lifecycleState(group), StartedAt: lifecycleStart(group),
		CompletedAt: lifecycleCompleted(group), RawEvents: append([]observability.DiagnosticEvent(nil), group.events...),
	}
	if current != nil {
		attempt.Status = current.Status
		attempt.ErrorCode = current.ErrorCode
		attempt.HTTPStatus = current.HTTPStatus
		attempt.DurationMS = current.DurationMS
	}
	return attempt
}

func toolFromGroup(group toolGroup) Tool {
	current := group.lifecycle.terminal
	if current == nil {
		current = group.lifecycle.started
	}
	tool := Tool{
		ID: group.id, Lifecycle: lifecycleState(group.lifecycle),
		StartedAt: lifecycleStart(group.lifecycle), CompletedAt: lifecycleCompleted(group.lifecycle),
		RawEvents: append([]observability.DiagnosticEvent(nil), group.lifecycle.events...),
	}
	if current != nil {
		tool.Name = current.ToolName
		tool.Status = current.Status
		tool.ErrorCode = current.ErrorCode
		tool.DurationMS = current.DurationMS
	}
	for _, event := range group.approvals {
		tool.RawEvents = append(tool.RawEvents, event)
		if event.Name == observability.ApprovalCompleted || event.Name == observability.ApprovalFailed {
			tool.ApprovalDurationMS += event.DurationMS
		}
	}
	tool.ExecutionDurationMS = max(0, tool.DurationMS-tool.ApprovalDurationMS)
	sort.SliceStable(tool.RawEvents, func(i, j int) bool {
		return tool.RawEvents[i].Timestamp.Before(tool.RawEvents[j].Timestamp)
	})
	return tool
}

func loadSnapshot(request *Request, run observability.DiagnosticRun, reader snapshot.Reader) {
	if reader == nil || strings.HasPrefix(request.ID, "turn:") || strings.HasPrefix(request.ID, "step:") {
		return
	}
	record, err := reader.Load(request.ID)
	if errors.Is(err, snapshot.ErrNotFound) || errors.Is(err, snapshot.ErrInvalidID) {
		return
	}
	if err != nil || record.SessionID != run.SessionID || record.RunID != run.ID {
		request.SnapshotState = SnapshotError
		return
	}
	if (request.TurnID != "" && record.TurnID != "" && request.TurnID != record.TurnID) ||
		(request.StepID != "" && record.StepID != "" && request.StepID != record.StepID) {
		request.SnapshotState = SnapshotError
		return
	}
	request.SnapshotState = SnapshotAvailable
	if request.TurnID == "" {
		request.TurnID = record.TurnID
	}
	if request.StepID == "" {
		request.StepID = record.StepID
	}
	request.CapturedAt = timePointer(record.CapturedAt)
	input := record.Input
	request.Input = &input
	request.Output = record.Output
	request.Attachments = append([]snapshot.Attachment(nil), record.Attachments...)
}

func attachSnapshotTools(requests []Request) {
	results := make(map[string]snapshot.Message)
	for _, request := range requests {
		if request.Input == nil {
			continue
		}
		for _, message := range request.Input.Messages {
			if message.Role == "toolResult" && message.ToolCallID != "" {
				results[message.ToolCallID] = message
			}
		}
	}
	for requestIndex := range requests {
		request := &requests[requestIndex]
		tools := make(map[string]*Tool, len(request.Tools))
		for index := range request.Tools {
			tools[request.Tools[index].ID] = &request.Tools[index]
		}
		if request.Output != nil {
			for _, content := range request.Output.Message.Content {
				if content.Type != "toolCall" || content.ToolCallID == "" {
					continue
				}
				tool := tools[content.ToolCallID]
				if tool == nil {
					status, lifecycle := "requested", "in-progress"
					var completedAt *time.Time
					if request.Status == "cancelled" || request.Output.StopReason == "aborted" {
						status, lifecycle, completedAt = "cancelled", "complete", request.CompletedAt
					} else if request.Status == "failed" || request.Output.StopReason == "error" {
						status, lifecycle, completedAt = "failed", "complete", request.CompletedAt
					}
					request.Tools = append(request.Tools, Tool{
						ID: content.ToolCallID, Name: content.ToolName,
						Status: status, Lifecycle: lifecycle,
						StartedAt: request.CompletedAtValue(), CompletedAt: completedAt,
						RawEvents: []observability.DiagnosticEvent{},
					})
					tool = &request.Tools[len(request.Tools)-1]
					tools[content.ToolCallID] = tool
				}
				tool.Arguments = content.Arguments
				if tool.Name == "" {
					tool.Name = content.ToolName
				}
			}
		}
		for index := range request.Tools {
			if result, found := results[request.Tools[index].ID]; found {
				copy := result
				request.Tools[index].Result = &copy
			}
		}
	}
}

func (request Request) CompletedAtValue() time.Time {
	if request.CompletedAt != nil {
		return *request.CompletedAt
	}
	return request.StartedAt
}

func latestHumanMessage(input snapshot.Input, attachments []snapshot.Attachment) string {
	attached := make(map[int]bool, len(attachments))
	for _, attachment := range attachments {
		attached[attachment.MessageIndex] = true
	}
	for index := len(input.Messages) - 1; index >= 0; index-- {
		message := input.Messages[index]
		if message.Role != "user" || attached[index] {
			continue
		}
		var parts []string
		for _, content := range message.Content {
			switch content.Type {
			case "text":
				parts = append(parts, content.Text)
			case "image":
				parts = append(parts, "[image]")
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	}
	return ""
}
