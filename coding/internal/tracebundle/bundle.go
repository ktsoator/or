// Package tracebundle assembles performance events and private request
// snapshots into one UI-facing diagnostic read model.
package tracebundle

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/requestsnapshot"
)

const CurrentVersion = 1

var ErrTaskNotFound = errors.New("trace task not found")

type SnapshotState string

const (
	SnapshotAvailable SnapshotState = "available"
	SnapshotMissing   SnapshotState = "missing"
	SnapshotError     SnapshotState = "error"
)

// Bundle is the complete diagnostic projection for one conversation.
type Bundle struct {
	Version        int       `json:"version"`
	GeneratedAt    time.Time `json:"generatedAt"`
	SessionID      string    `json:"sessionId"`
	SelectedTaskID string    `json:"selectedTaskId"`
	Tasks          []Task    `json:"tasks"`
}

// Task is one Prompt or Continue invocation inside a conversation.
type Task struct {
	ID                   string                          `json:"id"`
	Status               string                          `json:"status"`
	ErrorCode            string                          `json:"errorCode,omitempty"`
	Prompt               string                          `json:"prompt,omitempty"`
	StartedAt            time.Time                       `json:"startedAt"`
	UpdatedAt            time.Time                       `json:"updatedAt"`
	DurationMS           int64                           `json:"durationMs,omitempty"`
	TimeToFirstOutputMS  int64                           `json:"timeToFirstOutputMs,omitempty"`
	CheckpointDurationMS int64                           `json:"checkpointDurationMs,omitempty"`
	ToolDurationMS       int64                           `json:"toolDurationMs,omitempty"`
	ApprovalDurationMS   int64                           `json:"approvalDurationMs,omitempty"`
	Retries              int                             `json:"retries"`
	ContextRecoveries    int                             `json:"contextRecoveries"`
	InputTokens          int64                           `json:"inputTokens,omitempty"`
	OutputTokens         int64                           `json:"outputTokens,omitempty"`
	CacheReadTokens      int64                           `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens     int64                           `json:"cacheWriteTokens,omitempty"`
	TotalTokens          int64                           `json:"totalTokens,omitempty"`
	CostTotalUSD         float64                         `json:"costTotalUsd,omitempty"`
	Requests             []Request                       `json:"requests"`
	RawEvents            []observability.DiagnosticEvent `json:"rawEvents"`
	OmittedEvents        int                             `json:"omittedEvents,omitempty"`
}

// Request is one model step. It owns the response and tools produced by that
// response; its exact provider input remains available for explicit inspection.
type Request struct {
	ID                   string                          `json:"id"`
	Number               int                             `json:"number"`
	TurnID               string                          `json:"turnId,omitempty"`
	Status               string                          `json:"status,omitempty"`
	ErrorCode            string                          `json:"errorCode,omitempty"`
	Lifecycle            string                          `json:"lifecycle"`
	StartedAt            time.Time                       `json:"startedAt"`
	CompletedAt          *time.Time                      `json:"completedAt,omitempty"`
	DurationMS           int64                           `json:"durationMs,omitempty"`
	TimeToFirstOutputMS  int64                           `json:"timeToFirstOutputMs,omitempty"`
	CheckpointDurationMS int64                           `json:"checkpointDurationMs,omitempty"`
	Provider             string                          `json:"provider,omitempty"`
	Model                string                          `json:"model,omitempty"`
	InputTokens          int64                           `json:"inputTokens,omitempty"`
	InputUnknown         bool                            `json:"inputUnknown,omitempty"`
	OutputTokens         int64                           `json:"outputTokens,omitempty"`
	CacheReadTokens      int64                           `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens     int64                           `json:"cacheWriteTokens,omitempty"`
	TotalTokens          int64                           `json:"totalTokens,omitempty"`
	CostTotalUSD         float64                         `json:"costTotalUsd,omitempty"`
	Attempts             []Attempt                       `json:"attempts"`
	Checkpoints          []Checkpoint                    `json:"checkpoints"`
	Tools                []Tool                          `json:"tools"`
	SnapshotState        SnapshotState                   `json:"snapshotState"`
	CapturedAt           *time.Time                      `json:"capturedAt,omitempty"`
	Input                *requestsnapshot.Input          `json:"input,omitempty"`
	Output               *requestsnapshot.Output         `json:"output,omitempty"`
	Attachments          []requestsnapshot.Attachment    `json:"attachments,omitempty"`
	RawEvents            []observability.DiagnosticEvent `json:"rawEvents"`
}

type Attempt struct {
	Number      int                             `json:"number"`
	Status      string                          `json:"status,omitempty"`
	Lifecycle   string                          `json:"lifecycle"`
	StartedAt   time.Time                       `json:"startedAt"`
	CompletedAt *time.Time                      `json:"completedAt,omitempty"`
	DurationMS  int64                           `json:"durationMs,omitempty"`
	HTTPStatus  int                             `json:"httpStatus,omitempty"`
	ErrorCode   string                          `json:"errorCode,omitempty"`
	RawEvents   []observability.DiagnosticEvent `json:"rawEvents"`
}

type Checkpoint struct {
	Status      string    `json:"status,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DurationMS  int64     `json:"durationMs,omitempty"`
	ErrorCode   string    `json:"errorCode,omitempty"`
}

type Tool struct {
	ID                  string                          `json:"id"`
	Name                string                          `json:"name,omitempty"`
	Status              string                          `json:"status,omitempty"`
	ErrorCode           string                          `json:"errorCode,omitempty"`
	Lifecycle           string                          `json:"lifecycle"`
	StartedAt           time.Time                       `json:"startedAt"`
	CompletedAt         *time.Time                      `json:"completedAt,omitempty"`
	DurationMS          int64                           `json:"durationMs,omitempty"`
	ApprovalDurationMS  int64                           `json:"approvalDurationMs,omitempty"`
	ExecutionDurationMS int64                           `json:"executionDurationMs,omitempty"`
	Arguments           map[string]any                  `json:"arguments,omitempty"`
	Result              *requestsnapshot.Message        `json:"result,omitempty"`
	RawEvents           []observability.DiagnosticEvent `json:"rawEvents"`
}

type lifecycleGroup struct {
	started  *observability.DiagnosticEvent
	terminal *observability.DiagnosticEvent
	events   []observability.DiagnosticEvent
}

type requestGroup struct {
	id          string
	turnID      string
	lifecycle   lifecycleGroup
	events      []observability.DiagnosticEvent
	attempts    map[int]*lifecycleGroup
	checkpoints []observability.DiagnosticEvent
	tools       map[string]*toolGroup
}

type toolGroup struct {
	id        string
	lifecycle lifecycleGroup
	approvals []observability.DiagnosticEvent
}

// Build creates one conversation bundle from an already session-scoped
// diagnostic report. Missing or unreadable snapshots degrade individual
// requests instead of making the whole diagnostic view fail.
func Build(
	report observability.DiagnosticReport,
	sessionID, runID string,
	snapshots requestsnapshot.Reader,
) (Bundle, error) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	runs := sessionRuns(report.Runs, sessionID)
	if len(runs) == 0 {
		return Bundle{}, ErrTaskNotFound
	}
	selectedTaskID := runs[0].ID
	if runID != "" {
		selectedTaskID = ""
		for _, run := range runs {
			if run.ID == runID {
				selectedTaskID = run.ID
				break
			}
		}
		if selectedTaskID == "" {
			return Bundle{}, ErrTaskNotFound
		}
	}
	numbers := sessionRequestNumbers(runs, sessionID)
	tasks := make([]Task, 0, len(runs))
	for _, run := range runs {
		tasks = append(tasks, taskFromRun(run, buildRequests(run, numbers, snapshots)))
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].StartedAt.Before(tasks[j].StartedAt)
	})
	return Bundle{
		Version: CurrentVersion, GeneratedAt: report.GeneratedAt,
		SessionID: sessionID, SelectedTaskID: selectedTaskID, Tasks: tasks,
	}, nil
}

func sessionRuns(runs []observability.DiagnosticRun, sessionID string) []observability.DiagnosticRun {
	result := make([]observability.DiagnosticRun, 0, len(runs))
	for _, run := range runs {
		if run.SessionID != sessionID {
			continue
		}
		result = append(result, run)
	}
	return result
}

func taskFromRun(run observability.DiagnosticRun, requests []Request) Task {
	prompt := ""
	if len(requests) > 0 && requests[0].Input != nil {
		prompt = latestHumanMessage(*requests[0].Input, requests[0].Attachments)
	}
	return Task{
		ID: run.ID, Status: run.Status, ErrorCode: run.ErrorCode, Prompt: prompt,
		StartedAt: run.StartedAt, UpdatedAt: run.UpdatedAt, DurationMS: run.DurationMS,
		TimeToFirstOutputMS:  run.TimeToFirstOutputMS,
		CheckpointDurationMS: run.CheckpointDurationMS, ToolDurationMS: run.ToolDurationMS,
		ApprovalDurationMS: run.ApprovalDurationMS, Retries: run.Retries,
		ContextRecoveries: run.ContextRecoveries, InputTokens: run.InputTokens,
		OutputTokens: run.OutputTokens, CacheReadTokens: run.CacheReadTokens,
		CacheWriteTokens: run.CacheWriteTokens, TotalTokens: run.TotalTokens,
		CostTotalUSD: run.CostTotalUSD, Requests: requests,
		RawEvents:     append([]observability.DiagnosticEvent(nil), run.Events...),
		OmittedEvents: run.OmittedEvents,
	}
}

func sessionRequestNumbers(runs []observability.DiagnosticRun, sessionID string) map[string]int {
	type reference struct {
		id        string
		startedAt time.Time
	}
	var references []reference
	seen := make(map[string]bool)
	for _, run := range runs {
		if run.SessionID != sessionID {
			continue
		}
		for _, group := range groupRun(run) {
			if seen[group.id] {
				continue
			}
			seen[group.id] = true
			references = append(references, reference{
				id: group.id, startedAt: lifecycleStart(group.lifecycle),
			})
		}
	}
	sort.SliceStable(references, func(i, j int) bool {
		return references[i].startedAt.Before(references[j].startedAt)
	})
	result := make(map[string]int, len(references))
	for index, reference := range references {
		result[reference.id] = index + 1
	}
	return result
}

func buildRequests(
	run observability.DiagnosticRun,
	numbers map[string]int,
	snapshots requestsnapshot.Reader,
) []Request {
	groups := groupRun(run)
	requests := make([]Request, 0, len(groups))
	for index, group := range groups {
		request := requestFromGroup(group)
		request.Number = numbers[group.id]
		if request.Number == 0 {
			request.Number = index + 1
		}
		loadSnapshot(&request, run, snapshots)
		requests = append(requests, request)
	}
	attachSnapshotTools(requests)
	return requests
}

func groupRun(run observability.DiagnosticRun) []*requestGroup {
	turnRequests := make(map[string]string)
	for _, event := range run.Events {
		if event.ProviderRequestID != "" && event.TurnID != "" {
			turnRequests[event.TurnID] = event.ProviderRequestID
		}
	}
	groups := make(map[string]*requestGroup)
	toolFallbacks := make(map[string]string)
	toolCounter := 0
	for _, event := range run.Events {
		requestID := event.ProviderRequestID
		if requestID == "" {
			requestID = turnRequests[event.TurnID]
		}
		if requestID == "" && event.TurnID != "" && strings.HasPrefix(event.Name, "provider.") {
			requestID = "turn:" + event.TurnID
		}
		if requestID == "" {
			continue
		}
		group := groups[requestID]
		if group == nil {
			group = &requestGroup{
				id: requestID, turnID: event.TurnID,
				attempts: make(map[int]*lifecycleGroup), tools: make(map[string]*toolGroup),
			}
			groups[requestID] = group
		}
		if group.turnID == "" {
			group.turnID = event.TurnID
		}
		group.events = append(group.events, event)
		switch event.Name {
		case observability.ProviderStarted:
			group.lifecycle.events = append(group.lifecycle.events, event)
			group.lifecycle.started = eventPointer(event)
		case observability.ProviderCompleted, observability.ProviderFailed:
			group.lifecycle.events = append(group.lifecycle.events, event)
			group.lifecycle.terminal = eventPointer(event)
		case observability.HTTPAttemptStarted, observability.HTTPAttemptResponse:
			attempt := event.Attempt
			if attempt <= 0 {
				attempt = 1
			}
			attemptGroup := group.attempts[attempt]
			if attemptGroup == nil {
				attemptGroup = &lifecycleGroup{}
				group.attempts[attempt] = attemptGroup
			}
			attemptGroup.events = append(attemptGroup.events, event)
			if event.Name == observability.HTTPAttemptStarted {
				attemptGroup.started = eventPointer(event)
			} else {
				attemptGroup.terminal = eventPointer(event)
			}
		case observability.CheckpointCompleted, observability.CheckpointFailed:
			group.checkpoints = append(group.checkpoints, event)
		case observability.ToolStarted, observability.ToolCompleted, observability.ToolFailed,
			observability.ApprovalStarted, observability.ApprovalCompleted, observability.ApprovalFailed:
			toolID := event.ToolCallID
			scope := event.TurnID + ":" + event.ToolName
			if toolID == "" {
				toolID = toolFallbacks[scope]
				if toolID == "" || event.Name == observability.ToolStarted {
					toolCounter++
					toolID = "missing-tool-" + strconv.Itoa(toolCounter)
					toolFallbacks[scope] = toolID
				}
			}
			tool := group.tools[toolID]
			if tool == nil {
				tool = &toolGroup{id: toolID}
				group.tools[toolID] = tool
			}
			if strings.HasPrefix(event.Name, "tool.approval.") {
				tool.approvals = append(tool.approvals, event)
			} else {
				tool.lifecycle.events = append(tool.lifecycle.events, event)
				if event.Name == observability.ToolStarted {
					tool.lifecycle.started = eventPointer(event)
				} else {
					tool.lifecycle.terminal = eventPointer(event)
				}
			}
		}
	}
	result := make([]*requestGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return lifecycleStart(result[i].lifecycle).Before(lifecycleStart(result[j].lifecycle))
	})
	return result
}

func requestFromGroup(group *requestGroup) Request {
	terminal := group.lifecycle.terminal
	current := terminal
	if current == nil {
		current = group.lifecycle.started
	}
	request := Request{
		ID: group.id, TurnID: group.turnID, Lifecycle: lifecycleState(group.lifecycle),
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
	for number, group := range group.attempts {
		request.Attempts = append(request.Attempts, attemptFromGroup(number, *group))
	}
	sort.Slice(request.Attempts, func(i, j int) bool {
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

func attemptFromGroup(number int, group lifecycleGroup) Attempt {
	current := group.terminal
	if current == nil {
		current = group.started
	}
	attempt := Attempt{
		Number: number, Lifecycle: lifecycleState(group), StartedAt: lifecycleStart(group),
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

func loadSnapshot(request *Request, run observability.DiagnosticRun, reader requestsnapshot.Reader) {
	if reader == nil || strings.HasPrefix(request.ID, "turn:") {
		return
	}
	snapshot, err := reader.Load(request.ID)
	if errors.Is(err, requestsnapshot.ErrNotFound) || errors.Is(err, requestsnapshot.ErrInvalidID) {
		return
	}
	if err != nil || snapshot.SessionID != run.SessionID || snapshot.RunID != run.ID {
		request.SnapshotState = SnapshotError
		return
	}
	request.SnapshotState = SnapshotAvailable
	request.CapturedAt = timePointer(snapshot.CapturedAt)
	input := snapshot.Input
	request.Input = &input
	request.Output = snapshot.Output
	request.Attachments = append([]requestsnapshot.Attachment(nil), snapshot.Attachments...)
}

func attachSnapshotTools(requests []Request) {
	results := make(map[string]requestsnapshot.Message)
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

func latestHumanMessage(input requestsnapshot.Input, attachments []requestsnapshot.Attachment) string {
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

func lifecycleStart(group lifecycleGroup) time.Time {
	if group.started != nil {
		return group.started.Timestamp
	}
	if group.terminal != nil {
		return subtractDuration(group.terminal.Timestamp, group.terminal.DurationMS)
	}
	if len(group.events) > 0 {
		return group.events[0].Timestamp
	}
	return time.Time{}
}

func lifecycleCompleted(group lifecycleGroup) *time.Time {
	if group.terminal == nil {
		return nil
	}
	return timePointer(group.terminal.Timestamp)
}

func lifecycleState(group lifecycleGroup) string {
	switch {
	case group.started != nil && group.terminal != nil:
		return "complete"
	case group.terminal != nil:
		return "missing-start"
	default:
		return "in-progress"
	}
}

func subtractDuration(value time.Time, durationMS int64) time.Time {
	if value.IsZero() || durationMS <= 0 {
		return value
	}
	return value.Add(-time.Duration(durationMS) * time.Millisecond)
}

func eventPointer(event observability.DiagnosticEvent) *observability.DiagnosticEvent {
	copy := event
	return &copy
}

func timePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}
