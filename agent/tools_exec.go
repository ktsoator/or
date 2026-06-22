package agent

import (
	"encoding/json"
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
		message, terminate := e.runTool(current, assistant, toolCalls[index])
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
			result, isError := e.executePrepared(prepared[index])
			executed[index] = executedToolCall{result: result, isError: isError}
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
			message, terminate = e.finishError(prepared[index].call, prepared[index].errText)
		} else {
			message, terminate = e.finalize(current, assistant, prepared[index], executed[index].result, executed[index].isError)
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
		return e.finishError(call, prepared.errText)
	}
	result, isError := e.executePrepared(prepared)
	return e.finalize(current, assistant, prepared, result, isError)
}

// preparedToolCall is the result of preflighting one tool call. A non-empty
// errText means the call failed validation or was blocked and must not execute.
type preparedToolCall struct {
	call      llm.ToolCall
	tool      *AgentTool
	validated map[string]any
	rawArgs   json.RawMessage
	errText   string
}

// executedToolCall is the raw outcome of one Execute call.
type executedToolCall struct {
	result  ToolResult
	isError bool
}

// preflight resolves the tool, validates arguments, and runs BeforeToolCall. It
// does not execute the tool. It must run in source order because BeforeToolCall
// is a caller hook that should not be invoked concurrently.
func (e *engine) preflight(current Context, assistant llm.AssistantMessage, call llm.ToolCall) preparedToolCall {
	prepared := preparedToolCall{call: call}

	tool := findTool(current.Tools, call.Name)
	if tool == nil {
		prepared.errText = fmt.Sprintf("unknown tool %q", call.Name)
		return prepared
	}
	if tool.Execute == nil {
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
		prepared.errText = fmt.Sprintf("invalid tool arguments: %v", err)
		return prepared
	}

	if e.cfg.BeforeToolCall != nil {
		block, reason := e.cfg.BeforeToolCall(BeforeToolCallCtx{
			AssistantMessage: assistant,
			ToolCall:         call,
			Args:             validated,
			Context:          current,
		})
		if block {
			if reason == "" {
				reason = "tool call blocked"
			}
			prepared.errText = reason
			return prepared
		}
	}

	rawArgs, err := json.Marshal(validated)
	if err != nil {
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
func (e *engine) executePrepared(prepared preparedToolCall) (ToolResult, bool) {
	onUpdate := func(partial ToolResult) {
		e.emit(AgentEvent{
			Type:       ToolUpdate,
			ToolCallID: prepared.call.ID,
			ToolName:   prepared.call.Name,
			Args:       prepared.validated,
			Result:     partial,
		})
	}

	result, err := prepared.tool.Execute(e.ctx, prepared.call.ID, prepared.rawArgs, onUpdate)
	if err != nil {
		return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: err.Error()}}}, true
	}
	return result, false
}

// finalize applies AfterToolCall and emits the end-of-tool and result-message
// events. It must run in source order so AfterToolCall is not invoked
// concurrently.
func (e *engine) finalize(current Context, assistant llm.AssistantMessage, prepared preparedToolCall, result ToolResult, isError bool) (llm.ToolResultMessage, bool) {
	if e.cfg.AfterToolCall != nil {
		override := e.cfg.AfterToolCall(AfterToolCallCtx{
			AssistantMessage: assistant,
			ToolCall:         prepared.call,
			Args:             prepared.validated,
			Result:           result,
			IsError:          isError,
			Context:          current,
		})
		if override != nil {
			if override.Content != nil {
				result.Content = override.Content
			}
			if override.Details != nil {
				result.Details = override.Details
			}
			if override.IsError != nil {
				isError = *override.IsError
			}
			if override.Terminate != nil {
				result.Terminate = *override.Terminate
			}
		}
	}
	return e.finish(prepared.call, result, isError)
}

// finish emits the end-of-tool and result-message events and returns the result
// message with its terminate hint.
func (e *engine) finish(call llm.ToolCall, result ToolResult, isError bool) (llm.ToolResultMessage, bool) {
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
func (e *engine) finishError(call llm.ToolCall, text string) (llm.ToolResultMessage, bool) {
	return e.finish(call, ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: text}}}, true)
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
