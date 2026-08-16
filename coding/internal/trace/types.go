// Package trace assembles performance events and private request
// snapshots into one UI-facing diagnostic read model.
package trace

import (
	"errors"
	"time"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/snapshot"
)

const CurrentVersion = 2

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
	Page           PageInfo  `json:"page"`
}

// PageInfo describes the older-history page following this bundle.
type PageInfo struct {
	HasMore      bool   `json:"hasMore"`
	BeforeCursor string `json:"beforeCursor,omitempty"`
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
	StepID               string                          `json:"stepId,omitempty"`
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
	Input                *snapshot.Input                 `json:"input,omitempty"`
	Output               *snapshot.Output                `json:"output,omitempty"`
	Attachments          []snapshot.Attachment           `json:"attachments,omitempty"`
	RawEvents            []observability.DiagnosticEvent `json:"rawEvents"`
}

type Attempt struct {
	ID          string                          `json:"id"`
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
	Result              *snapshot.Message               `json:"result,omitempty"`
	RawEvents           []observability.DiagnosticEvent `json:"rawEvents"`
}
