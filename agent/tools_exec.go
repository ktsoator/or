package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ktsoator/or/llm"
)

// executeToolCalls runs a batch of tool calls and returns one result message per
// call, in source order. The batch terminates the run only when every result
// sets Terminate.
//
// A batch runs concurrently by default. It runs sequentially when ToolExecution
// is ExecutionSequential or when any tool in the batch declares
// ExecutionSequential. In a concurrent batch only the tools' Execute functions
// run in parallel: ToolStart events and BeforeToolCall run in source order
// before execution, and AfterToolCall, ToolEnd, and result-message events run in
// source order after the whole batch finishes. Hooks are therefore never called
// concurrently, while result events stay deterministic.
func (e *engine) executeToolCalls(current Context, assistant llm.AssistantMessage, toolCalls []llm.ToolCall) ([]llm.ToolResultMessage, bool) {
	if e.runsConcurrently(current, toolCalls) {
		return e.executeParallel(current, assistant, toolCalls)
	}
	return e.executeSequential(current, assistant, toolCalls)
}

// runsConcurrently reports whether a batch may run its tools in parallel. A
// sequential loop default or any sequential tool forces the whole batch
// sequential.
func (e *engine) runsConcurrently(current Context, toolCalls []llm.ToolCall) bool {
	if e.cfg.ToolExecution == ExecutionSequential {
		return false
	}
	for index := range toolCalls {
		if tool := findTool(current.Tools, toolCalls[index].Name); tool != nil && tool.ExecutionMode == ExecutionSequential {
			return false
		}
	}
	return true
}

func (e *engine) executeSequential(current Context, assistant llm.AssistantMessage, toolCalls []llm.ToolCall) ([]llm.ToolResultMessage, bool) {
	messages := make([]llm.ToolResultMessage, 0, len(toolCalls))
	allTerminate := true
	for index := range toolCalls {
		call := toolCalls[index]
		var message llm.ToolResultMessage
		var terminate bool
		if err := e.ctx.Err(); err != nil {
			// The run was cancelled mid-batch. Skip executing the remaining
			// tools, but still answer each call so every tool call has a result
			// and the transcript stays valid for any later request.
			e.emit(AgentEvent{Type: ToolStart, ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})
			message, terminate = e.finishError(call, ToolOutcomeCancelled, "tool_execution_cancelled", "tool execution aborted")
		} else {
			message, terminate = e.runTool(current, assistant, call)
		}
		messages = append(messages, message)
		if !terminate {
			allTerminate = false
		}
	}
	return messages, allTerminate && len(messages) > 0
}

func (e *engine) executeParallel(current Context, assistant llm.AssistantMessage, toolCalls []llm.ToolCall) ([]llm.ToolResultMessage, bool) {
	// Preflight in source order: emit ToolStart and run BeforeToolCall.
	prepared := make([]preparedToolCall, len(toolCalls))
	for index := range toolCalls {
		call := toolCalls[index]
		e.emit(AgentEvent{Type: ToolStart, ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})
		prepared[index] = e.preflight(current, assistant, call)
	}

	// Execute the tools that passed preflight concurrently.
	executed := make([]executedToolCall, len(toolCalls))
	var wait sync.WaitGroup
	for index := range prepared {
		if prepared[index].errText != "" {
			continue
		}
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			executed[index] = executedToolCall{result: e.executePrepared(prepared[index])}
		}(index)
	}
	wait.Wait()

	// Finalize in source order: run AfterToolCall and emit end events.
	messages := make([]llm.ToolResultMessage, 0, len(toolCalls))
	allTerminate := true
	for index := range toolCalls {
		var message llm.ToolResultMessage
		var terminate bool
		if prepared[index].errText != "" {
			message, terminate = e.finishError(prepared[index].call, ToolOutcomeFailed, prepared[index].errorCode, prepared[index].errText)
		} else {
			message, terminate = e.finalize(current, assistant, prepared[index], executed[index].result)
		}
		messages = append(messages, message)
		if !terminate {
			allTerminate = false
		}
	}
	return messages, allTerminate && len(messages) > 0
}

// runTool validates, optionally blocks, executes, and finalizes one tool call.
// It always returns a result message; failures become error results so one
// failing tool never aborts the run.
func (e *engine) runTool(current Context, assistant llm.AssistantMessage, call llm.ToolCall) (llm.ToolResultMessage, bool) {
	e.emit(AgentEvent{Type: ToolStart, ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})

	prepared := e.preflight(current, assistant, call)
	if prepared.errText != "" {
		return e.finishError(call, ToolOutcomeFailed, prepared.errorCode, prepared.errText)
	}
	result := e.executePrepared(prepared)
	return e.finalize(current, assistant, prepared, result)
}

// preparedToolCall is the result of preflighting one tool call. A non-empty
// errText means the call failed validation or was blocked and must not execute.
type preparedToolCall struct {
	call      llm.ToolCall
	tool      *AgentTool
	validated map[string]any
	rawArgs   json.RawMessage
	errorCode string
	errText   string
}

// executedToolCall is the raw outcome of one Execute call.
type executedToolCall struct {
	result ToolResult
}

// preflight resolves the tool, validates arguments, and runs BeforeToolCall. It
// does not execute the tool. It must run in source order because BeforeToolCall
// is a caller hook that should not be invoked concurrently.
func (e *engine) preflight(current Context, assistant llm.AssistantMessage, call llm.ToolCall) preparedToolCall {
	prepared := preparedToolCall{call: call}

	tool := findTool(current.Tools, call.Name)
	if tool == nil {
		prepared.errorCode = "unknown_tool"
		prepared.errText = fmt.Sprintf("unknown tool %q", call.Name)
		return prepared
	}
	if tool.Execute == nil {
		prepared.errorCode = "tool_unavailable"
		prepared.errText = fmt.Sprintf("tool %q has no Execute function", call.Name)
		return prepared
	}

	// Let the tool rewrite raw arguments before validation.
	if tool.PrepareArguments != nil {
		call.Arguments = tool.PrepareArguments(call.Arguments)
		prepared.call = call
	}

	validated, err := llm.ValidateToolArguments(tool.Definition, call)
	if err != nil {
		prepared.errorCode = "invalid_arguments"
		prepared.errText = fmt.Sprintf("invalid tool arguments: %v", err)
		return prepared
	}

	if e.cfg.BeforeToolCall != nil {
		block, reason := e.cfg.BeforeToolCall(BeforeToolCallCtx{
			RunContext:       e.ctx,
			AssistantMessage: assistant,
			ToolCall:         call,
			Args:             validated,
			Context:          current,
		})
		if block {
			if reason == "" {
				reason = "tool call blocked"
			}
			prepared.errorCode = "tool_blocked"
			prepared.errText = reason
			return prepared
		}
	}

	rawArgs, err := json.Marshal(validated)
	if err != nil {
		prepared.errorCode = "argument_encoding_failed"
		prepared.errText = fmt.Sprintf("encode tool arguments: %v", err)
		return prepared
	}

	prepared.tool = tool
	prepared.validated = validated
	prepared.rawArgs = rawArgs
	return prepared
}

// executePrepared runs a preflighted tool. It is safe to call concurrently for
// distinct calls: it reads only the prepared value and emits through the event
// channel, which is concurrency-safe.
func (e *engine) executePrepared(prepared preparedToolCall) (result ToolResult) {
	// A panicking tool becomes an error result rather than crashing the process.
	// This recover lives here, not only on the loop goroutine, because parallel
	// batches run Execute on separate goroutines that the loop cannot recover.
	defer func() {
		if r := recover(); r != nil {
			result = ToolResult{
				Content: []llm.ToolResultContent{&llm.TextContent{Text: fmt.Sprintf("tool %q panicked: %v", prepared.call.Name, r)}},
				Outcome: ToolOutcome{Status: ToolOutcomeFailed, ErrorCode: "tool_panicked"},
			}
		}
	}()

	var progressMu sync.Mutex
	acceptingProgress := true
	defer func() {
		progressMu.Lock()
		acceptingProgress = false
		progressMu.Unlock()
	}()
	onProgress := func(progress ToolProgress) {
		progressMu.Lock()
		defer progressMu.Unlock()
		if !acceptingProgress {
			return
		}
		e.emit(AgentEvent{
			Type:       ToolUpdate,
			ToolCallID: prepared.call.ID,
			ToolName:   prepared.call.Name,
			Args:       prepared.validated,
			Progress:   progress,
		})
	}

	out, err := prepared.tool.Execute(e.ctx, prepared.call.ID, prepared.rawArgs, onProgress)
	if err != nil {
		// Preserve the tool's own content and structured data when it supplied
		// them, then normalize the Go error into the shared outcome contract.
		if out.Content == nil {
			out.Content = []llm.ToolResultContent{&llm.TextContent{Text: err.Error()}}
		}
		out.Outcome = outcomeForError(out.Outcome, err)
		return out
	}
	out.Outcome = normalizeOutcome(out.Outcome)
	return out
}

// finalize applies AfterToolCall and emits the end-of-tool and result-message
// events. It must run in source order so AfterToolCall is not invoked
// concurrently.
func (e *engine) finalize(current Context, assistant llm.AssistantMessage, prepared preparedToolCall, result ToolResult) (llm.ToolResultMessage, bool) {
	if e.cfg.AfterToolCall != nil {
		override := e.cfg.AfterToolCall(AfterToolCallCtx{
			AssistantMessage: assistant,
			ToolCall:         prepared.call,
			Args:             prepared.validated,
			Result:           result,
			Context:          current,
		})
		if override != nil {
			if override.Content != nil {
				result.Content = override.Content
			}
			if override.Outcome != nil {
				result.Outcome = normalizeOutcome(*override.Outcome)
			}
			if override.Terminate != nil {
				result.Terminate = *override.Terminate
			}
		}
	}
	return e.finish(prepared.call, result)
}

// finish emits the end-of-tool and result-message events and returns the result
// message with its terminate hint.
func (e *engine) finish(call llm.ToolCall, result ToolResult) (llm.ToolResultMessage, bool) {
	result.Outcome = normalizeOutcome(result.Outcome)
	isError := result.Outcome.Failed()
	message := llm.ToolResultMessage{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		IsError:    isError,
		Content:    result.Content,
	}
	e.emit(AgentEvent{Type: ToolEnd, ToolCallID: call.ID, ToolName: call.Name, Result: result, IsError: isError})
	e.emit(AgentEvent{Type: MessageStart, Message: FromLLM(&message)})
	e.emit(AgentEvent{Type: MessageEnd, Message: FromLLM(&message)})
	return message, result.Terminate
}

// finishError finalizes a tool call that failed before or during execution. An
// error result never terminates the run.
func (e *engine) finishError(call llm.ToolCall, status ToolOutcomeStatus, code, text string) (llm.ToolResultMessage, bool) {
	return e.finish(call, ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: ToolOutcome{Status: status, ErrorCode: code},
	})
}

func normalizeOutcome(outcome ToolOutcome) ToolOutcome {
	switch outcome.Status {
	case "":
		outcome.Status = ToolOutcomeSuccess
	case ToolOutcomeSuccess:
	case ToolOutcomeFailed:
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "tool_failed"
		}
	case ToolOutcomeCancelled:
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "tool_cancelled"
		}
	case ToolOutcomeTimeout:
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "tool_timeout"
		}
	default:
		outcome.Status = ToolOutcomeFailed
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "invalid_tool_outcome"
		}
	}
	return outcome
}

func outcomeForError(outcome ToolOutcome, err error) ToolOutcome {
	outcome = normalizeOutcome(outcome)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		outcome.Status = ToolOutcomeTimeout
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "tool_execution_timeout"
		}
	case errors.Is(err, context.Canceled):
		outcome.Status = ToolOutcomeCancelled
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "tool_execution_cancelled"
		}
	default:
		outcome.Status = ToolOutcomeFailed
		if outcome.ErrorCode == "" {
			outcome.ErrorCode = "tool_execution_failed"
		}
	}
	return outcome
}

func findTool(tools []AgentTool, name string) *AgentTool {
	for index := range tools {
		if tools[index].Definition.Name == name {
			return &tools[index]
		}
	}
	return nil
}

func toolDefinitions(tools []AgentTool) []llm.ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	definitions := make([]llm.ToolDefinition, len(tools))
	for index := range tools {
		definitions[index] = tools[index].Definition
	}
	return definitions
}

func assistantToolCalls(message llm.AssistantMessage) []llm.ToolCall {
	var calls []llm.ToolCall
	for _, content := range message.Content {
		if call, ok := content.(*llm.ToolCall); ok && call != nil {
			calls = append(calls, *call)
		}
	}
	return calls
}
