package trace

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
)

type lifecycleGroup struct {
	started  *observability.DiagnosticEvent
	terminal *observability.DiagnosticEvent
	events   []observability.DiagnosticEvent
}

type requestGroup struct {
	id          string
	turnID      string
	stepID      string
	lifecycle   lifecycleGroup
	events      []observability.DiagnosticEvent
	attempts    map[string]*attemptGroup
	checkpoints []observability.DiagnosticEvent
	tools       map[string]*toolGroup
}

type attemptGroup struct {
	id        string
	number    int
	lifecycle lifecycleGroup
}

type toolGroup struct {
	id        string
	lifecycle lifecycleGroup
	approvals []observability.DiagnosticEvent
}

func groupRun(run observability.DiagnosticRun) []*requestGroup {
	stepRequests := make(map[string]string)
	legacyTurnRequests := make(map[string]string)
	for _, event := range run.Events {
		if event.ProviderRequestID == "" {
			continue
		}
		if event.StepID != "" {
			stepRequests[event.StepID] = event.ProviderRequestID
		} else if event.TurnID != "" {
			legacyTurnRequests[event.TurnID] = event.ProviderRequestID
		}
	}
	groups := make(map[string]*requestGroup)
	toolFallbacks := make(map[string]string)
	toolCounter := 0
	for _, event := range run.Events {
		requestID := event.ProviderRequestID
		if requestID == "" && event.StepID != "" {
			requestID = stepRequests[event.StepID]
		}
		if requestID == "" && event.StepID == "" {
			requestID = legacyTurnRequests[event.TurnID]
		}
		if requestID == "" && strings.HasPrefix(event.Name, "provider.") {
			if event.StepID != "" {
				requestID = "step:" + event.StepID
			} else if event.TurnID != "" {
				requestID = "turn:" + event.TurnID
			}
		}
		if requestID == "" {
			continue
		}
		group := groups[requestID]
		if group == nil {
			group = &requestGroup{
				id: requestID, turnID: event.TurnID, stepID: event.StepID,
				attempts: make(map[string]*attemptGroup), tools: make(map[string]*toolGroup),
			}
			groups[requestID] = group
		}
		if group.turnID == "" {
			group.turnID = event.TurnID
		}
		if group.stepID == "" {
			group.stepID = event.StepID
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
			attemptNumber := event.Attempt
			if attemptNumber <= 0 {
				attemptNumber = 1
			}
			attemptID := event.AttemptID
			if attemptID == "" {
				attemptID = "attempt:" + strconv.Itoa(attemptNumber)
			}
			attempt := group.attempts[attemptID]
			if attempt == nil {
				attempt = &attemptGroup{id: attemptID, number: attemptNumber}
				group.attempts[attemptID] = attempt
			}
			attempt.lifecycle.events = append(attempt.lifecycle.events, event)
			if event.Name == observability.HTTPAttemptStarted {
				attempt.lifecycle.started = eventPointer(event)
			} else {
				attempt.lifecycle.terminal = eventPointer(event)
			}
		case observability.CheckpointCompleted, observability.CheckpointFailed:
			group.checkpoints = append(group.checkpoints, event)
		case observability.ToolStarted, observability.ToolCompleted, observability.ToolFailed,
			observability.ApprovalStarted, observability.ApprovalCompleted, observability.ApprovalFailed:
			toolID := event.ToolCallID
			scope := event.StepID + ":" + event.TurnID + ":" + event.ToolName
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
