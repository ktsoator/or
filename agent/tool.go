package agent

import (
	"context"
	"encoding/json"

	"github.com/ktsoator/or/llm"
)

// ToolOutcomeStatus is the terminal state of one tool execution.
type ToolOutcomeStatus string

const (
	ToolOutcomeSuccess   ToolOutcomeStatus = "success"
	ToolOutcomeFailed    ToolOutcomeStatus = "failed"
	ToolOutcomeCancelled ToolOutcomeStatus = "cancelled"
	ToolOutcomeTimeout   ToolOutcomeStatus = "timeout"
)

// ToolOutcome is the machine-readable result of one tool execution. Content is
// still what the model reads; Outcome is what runtimes and product shells use
// for status, error handling, and structured rendering.
type ToolOutcome struct {
	Status    ToolOutcomeStatus
	ErrorCode string
	ExitCode  *int
	Data      any
}

// Failed reports whether the outcome should be sent to the model as an error
// tool result. The zero value is a successful outcome for backwards-compatible
// tool implementations.
func (o ToolOutcome) Failed() bool {
	return o.Status != "" && o.Status != ToolOutcomeSuccess
}

// ToolProgress is a non-terminal update emitted while a tool is running. It
// cannot report completion, failure, cancellation, timeout, or termination;
// those belong exclusively to the ToolResult returned by Execute.
type ToolProgress struct {
	// Content is incremental text or image content for progress consumers.
	Content []llm.ToolResultContent
	// Data is arbitrary structured progress data for runtimes and product shells.
	Data any
}

// ToolResult is what a tool returns to the model, with a machine-readable
// outcome and an optional early-termination hint.
type ToolResult struct {
	// Content is the text or image content returned to the model.
	Content []llm.ToolResultContent
	// Outcome is the source of truth for terminal status and structured data. An
	// empty Status is normalized to success by the execution engine.
	Outcome ToolOutcome
	// Terminate hints that the run should stop after the current tool batch. A
	// batch stops the run only when every result in it sets Terminate.
	Terminate bool
}

// ExecutionMode selects whether a tool batch runs sequentially or in parallel.
type ExecutionMode string

const (
	// ExecutionParallel runs a batch's tools concurrently. It is the default.
	ExecutionParallel ExecutionMode = "parallel"
	// ExecutionSequential runs a batch's tools one at a time.
	ExecutionSequential ExecutionMode = "sequential"
)

// AgentTool is a tool the model may call during a run.
type AgentTool struct {
	// Definition is the schema and description advertised to the model.
	Definition llm.ToolDefinition
	// Label is an optional human-readable name for UI display. It is metadata for
	// callers and does not affect execution.
	Label string
	// PrepareArguments optionally rewrites the raw tool-call arguments before
	// schema validation, for tolerating provider quirks or filling defaults. It
	// returns the arguments to validate and execute. Nil leaves them unchanged.
	PrepareArguments func(arguments map[string]any) map[string]any
	// Execute runs the tool. It reports failure by returning an error, which the
	// engine turns into an error tool result so one failing tool does not abort
	// the run. onProgress emits non-terminal updates and is valid only for the
	// duration of the call; updates after Execute returns are ignored.
	Execute func(ctx context.Context, callID string, args json.RawMessage, onProgress func(ToolProgress)) (ToolResult, error)
	// ExecutionMode overrides the loop default for this tool. Empty inherits it.
	ExecutionMode ExecutionMode
}
