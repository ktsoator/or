package trace

import (
	"sort"
	"strings"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/snapshot"
)

// Build creates one conversation bundle from an already session-scoped
// diagnostic report. Missing or unreadable snapshots degrade individual
// requests instead of making the whole diagnostic view fail.
func Build(
	report observability.DiagnosticReport,
	sessionID, runID string,
	snapshots snapshot.Reader,
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
	snapshots snapshot.Reader,
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
