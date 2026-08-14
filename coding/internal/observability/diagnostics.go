package observability

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultDiagnosticRunLimit   = 50
	DefaultDiagnosticEventLimit = 500
	MaximumDiagnosticRunLimit   = 100
	MaximumDiagnosticEventLimit = 1000
)

// DiagnosticQuery bounds and optionally scopes one read of the local log.
type DiagnosticQuery struct {
	SessionID    string
	RunLimit     int
	EventsPerRun int
}

// DiagnosticReport is the privacy-safe, UI-facing projection of local events.
type DiagnosticReport struct {
	Runs        []DiagnosticRun `json:"runs"`
	GeneratedAt time.Time       `json:"generatedAt"`
}

// DiagnosticRun summarizes one user-visible run and carries its event timeline.
type DiagnosticRun struct {
	ID                   string            `json:"id"`
	SessionID            string            `json:"sessionId"`
	Status               string            `json:"status"`
	ErrorCode            string            `json:"errorCode,omitempty"`
	StartedAt            time.Time         `json:"startedAt"`
	UpdatedAt            time.Time         `json:"updatedAt"`
	DurationMS           int64             `json:"durationMs,omitempty"`
	TimeToFirstOutputMS  int64             `json:"timeToFirstOutputMs,omitempty"`
	CheckpointDurationMS int64             `json:"checkpointDurationMs,omitempty"`
	ToolDurationMS       int64             `json:"toolDurationMs,omitempty"`
	ApprovalDurationMS   int64             `json:"approvalDurationMs,omitempty"`
	ProviderRequests     int               `json:"providerRequests"`
	ToolCalls            int               `json:"toolCalls"`
	ApprovalRequests     int               `json:"approvalRequests"`
	Retries              int               `json:"retries"`
	ContextRecoveries    int               `json:"contextRecoveries"`
	InputTokens          int64             `json:"inputTokens,omitempty"`
	OutputTokens         int64             `json:"outputTokens,omitempty"`
	CacheReadTokens      int64             `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens     int64             `json:"cacheWriteTokens,omitempty"`
	TotalTokens          int64             `json:"totalTokens,omitempty"`
	CostTotalUSD         float64           `json:"costTotalUsd,omitempty"`
	Events               []DiagnosticEvent `json:"events"`
	OmittedEvents        int               `json:"omittedEvents,omitempty"`
}

// DiagnosticEvent contains only fields approved for the observability schema.
type DiagnosticEvent struct {
	Name                string    `json:"name"`
	Timestamp           time.Time `json:"timestamp"`
	TurnID              string    `json:"turnId,omitempty"`
	ProviderRequestID   string    `json:"providerRequestId,omitempty"`
	ToolCallID          string    `json:"toolCallId,omitempty"`
	ToolName            string    `json:"toolName,omitempty"`
	Status              string    `json:"status,omitempty"`
	ErrorCode           string    `json:"errorCode,omitempty"`
	Reason              string    `json:"reason,omitempty"`
	DurationMS          int64     `json:"durationMs,omitempty"`
	TimeToFirstOutputMS int64     `json:"timeToFirstOutputMs,omitempty"`
	Provider            string    `json:"provider,omitempty"`
	Model               string    `json:"model,omitempty"`
	Attempt             int       `json:"attempt,omitempty"`
	HTTPStatus          int       `json:"httpStatus,omitempty"`
	InputTokens         int64     `json:"inputTokens,omitempty"`
	InputUnknown        bool      `json:"inputUnknown,omitempty"`
	OutputTokens        int64     `json:"outputTokens,omitempty"`
	CacheReadTokens     int64     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens    int64     `json:"cacheWriteTokens,omitempty"`
	TotalTokens         int64     `json:"totalTokens,omitempty"`
	CostTotalUSD        float64   `json:"costTotalUsd,omitempty"`
}

type storedEvent struct {
	Name                string    `json:"event"`
	Timestamp           time.Time `json:"time"`
	SessionID           string    `json:"session_id"`
	RunID               string    `json:"run_id"`
	TurnID              string    `json:"turn_id"`
	ProviderRequestID   string    `json:"provider_request_id"`
	ToolCallID          string    `json:"tool_call_id"`
	ToolName            string    `json:"tool_name"`
	Status              string    `json:"status"`
	ErrorCode           string    `json:"error_code"`
	Reason              string    `json:"reason"`
	StartedAt           time.Time `json:"started_at"`
	DurationMS          int64     `json:"duration_ms"`
	TimeToFirstOutputMS int64     `json:"time_to_first_output_ms"`
	Provider            string    `json:"provider"`
	Model               string    `json:"model"`
	Attempt             int       `json:"attempt"`
	HTTPStatus          int       `json:"http_status"`
	InputTokens         int64     `json:"input_tokens"`
	InputUnknown        bool      `json:"input_unknown"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheWriteTokens    int64     `json:"cache_write_tokens"`
	TotalTokens         int64     `json:"total_tokens"`
	CostTotalUSD        float64   `json:"cost_total_usd"`
}

// ReadDiagnosticReport reads the active log and its numeric rotations. A
// partially written final line is ignored so observing an active runtime is
// harmless.
func ReadDiagnosticReport(path string, query DiagnosticQuery) (DiagnosticReport, error) {
	query.RunLimit = boundedLimit(
		query.RunLimit, DefaultDiagnosticRunLimit, MaximumDiagnosticRunLimit,
	)
	query.EventsPerRun = boundedLimit(
		query.EventsPerRun, DefaultDiagnosticEventLimit, MaximumDiagnosticEventLimit,
	)
	runs := make(map[string]*DiagnosticRun)
	for _, candidate := range diagnosticLogPaths(path) {
		if err := readDiagnosticFile(candidate, strings.TrimSpace(query.SessionID), runs); err != nil {
			return DiagnosticReport{}, err
		}
	}

	generatedAt := time.Now().UTC()
	result := make([]DiagnosticRun, 0, len(runs))
	for _, run := range runs {
		if run.Status == "running" && !run.StartedAt.IsZero() {
			run.DurationMS = generatedAt.Sub(run.StartedAt).Milliseconds()
		}
		sort.SliceStable(run.Events, func(i, j int) bool {
			return run.Events[i].Timestamp.Before(run.Events[j].Timestamp)
		})
		if len(run.Events) > query.EventsPerRun {
			run.OmittedEvents = len(run.Events) - query.EventsPerRun
			run.Events = append([]DiagnosticEvent(nil), run.Events[len(run.Events)-query.EventsPerRun:]...)
		}
		result = append(result, *run)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	if len(result) > query.RunLimit {
		result = result[:query.RunLimit]
	}
	if result == nil {
		result = []DiagnosticRun{}
	}
	return DiagnosticReport{Runs: result, GeneratedAt: generatedAt}, nil
}

func boundedLimit(value, fallback, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func diagnosticLogPaths(path string) []string {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return []string{path}
	}
	base := filepath.Base(path)
	type rotatedPath struct {
		path  string
		index int
	}
	var rotated []rotatedPath
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, base+".") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(name, base+"."))
		if err == nil && index > 0 {
			rotated = append(rotated, rotatedPath{
				path: filepath.Join(filepath.Dir(path), name), index: index,
			})
		}
	}
	sort.Slice(rotated, func(i, j int) bool { return rotated[i].index > rotated[j].index })
	paths := make([]string, 0, len(rotated)+1)
	for _, candidate := range rotated {
		paths = append(paths, candidate.path)
	}
	return append(paths, path)
}

func readDiagnosticFile(path, sessionID string, runs map[string]*DiagnosticRun) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var event storedEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.RunID == "" || (sessionID != "" && event.SessionID != sessionID) {
			continue
		}
		if !diagnosticEventNames[event.Name] {
			continue
		}
		event.Reason = diagnosticReason(event.Reason)
		addStoredEvent(runs, event)
	}
	return scanner.Err()
}

func addStoredEvent(runs map[string]*DiagnosticRun, event storedEvent) {
	run := runs[event.RunID]
	if run == nil {
		run = &DiagnosticRun{ID: event.RunID, SessionID: event.SessionID, Status: "running"}
		runs[event.RunID] = run
	}
	if run.SessionID == "" {
		run.SessionID = event.SessionID
	}
	if run.StartedAt.IsZero() || (!event.StartedAt.IsZero() && event.StartedAt.Before(run.StartedAt)) {
		run.StartedAt = event.StartedAt
	}
	if run.StartedAt.IsZero() || event.Timestamp.Before(run.StartedAt) {
		run.StartedAt = event.Timestamp
	}
	if event.Timestamp.After(run.UpdatedAt) {
		run.UpdatedAt = event.Timestamp
	}
	if event.Name == RunCompleted || event.Name == RunFailed {
		run.Status = event.Status
		run.ErrorCode = event.ErrorCode
		run.DurationMS = event.DurationMS
	}
	switch event.Name {
	case ProviderStarted:
		run.ProviderRequests++
	case ProviderCompleted, ProviderFailed:
		if run.TimeToFirstOutputMS == 0 && event.TimeToFirstOutputMS > 0 {
			run.TimeToFirstOutputMS = event.TimeToFirstOutputMS
		}
		run.InputTokens += event.InputTokens
		run.OutputTokens += event.OutputTokens
		run.CacheReadTokens += event.CacheReadTokens
		run.CacheWriteTokens += event.CacheWriteTokens
		run.TotalTokens += event.TotalTokens
		run.CostTotalUSD += event.CostTotalUSD
	case CheckpointCompleted, CheckpointFailed:
		run.CheckpointDurationMS += event.DurationMS
	case ToolStarted:
		run.ToolCalls++
	case ToolCompleted, ToolFailed:
		run.ToolDurationMS += event.DurationMS
	case ApprovalStarted:
		run.ApprovalRequests++
	case ApprovalCompleted, ApprovalFailed:
		run.ApprovalDurationMS += event.DurationMS
	case TurnDiscarded:
		if event.Reason == "retry" {
			run.Retries++
		} else if event.Reason == "context_overflow" {
			run.ContextRecoveries++
		}
	}
	run.Events = append(run.Events, DiagnosticEvent{
		Name: event.Name, Timestamp: event.Timestamp,
		TurnID: event.TurnID, ProviderRequestID: event.ProviderRequestID,
		ToolCallID: event.ToolCallID, ToolName: event.ToolName,
		Status: event.Status, ErrorCode: event.ErrorCode, Reason: event.Reason,
		DurationMS: event.DurationMS, TimeToFirstOutputMS: event.TimeToFirstOutputMS,
		Provider: event.Provider, Model: event.Model,
		Attempt: event.Attempt, HTTPStatus: event.HTTPStatus,
		InputTokens: event.InputTokens, InputUnknown: event.InputUnknown,
		OutputTokens:    event.OutputTokens,
		CacheReadTokens: event.CacheReadTokens, CacheWriteTokens: event.CacheWriteTokens,
		TotalTokens: event.TotalTokens, CostTotalUSD: event.CostTotalUSD,
	})
}

var diagnosticEventNames = map[string]bool{
	RunStarted: true, RunCompleted: true, RunFailed: true,
	TurnStarted: true, TurnCompleted: true, TurnDiscarded: true,
	ProviderStarted: true, ProviderCompleted: true, ProviderFailed: true,
	HTTPAttemptStarted: true, HTTPAttemptResponse: true,
	CheckpointCompleted: true, CheckpointFailed: true,
	ToolStarted: true, ToolCompleted: true, ToolFailed: true,
	ApprovalStarted: true, ApprovalCompleted: true, ApprovalFailed: true,
}

func diagnosticReason(reason string) string {
	switch reason {
	case "retry", "context_overflow", "persistence_failure":
		return reason
	default:
		return ""
	}
}
