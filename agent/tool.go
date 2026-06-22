package agent

import (
	"context"
	"encoding/json"

	"github.com/ktsoator/or/llm"
)

// ToolResult is what a tool returns to the model, with optional structured
// details for logging or UI and an optional early-termination hint.
type ToolResult struct {
	// Content is the text or image content returned to the model.
	Content []llm.ToolResultContent
	// Details is arbitrary structured data for logs or UI rendering.
	Details any
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
	// the run. onUpdate streams partial results and is valid only for the
	// duration of the call.
	Execute func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(ToolResult)) (ToolResult, error)
	// ExecutionMode overrides the loop default for this tool. Empty inherits it.
	ExecutionMode ExecutionMode
}
