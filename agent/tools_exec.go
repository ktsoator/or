package agent

import (
	"encoding/json"
	"fmt"

	"github.com/ktsoator/or/llm"
)

// executeToolCalls runs a batch of tool calls and returns one result message per
// call. The batch terminates the run only when every result sets Terminate.
//
// Tools currently run sequentially regardless of ExecutionMode. Concurrent
// execution for the parallel mode is a planned follow-up; the mode field is
// accepted now so the API does not change when it lands.
func (e *engine) executeToolCalls(current Context, assistant llm.AssistantMessage, toolCalls []llm.ToolCall) ([]llm.ToolResultMessage, bool) {
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

// runTool validates, optionally blocks, executes, and finalizes one tool call.
// It always returns a result message; failures become error results so one
// failing tool never aborts the run.
func (e *engine) runTool(current Context, assistant llm.AssistantMessage, call llm.ToolCall) (llm.ToolResultMessage, bool) {
	e.emit(AgentEvent{Type: ToolStart, ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments})

	tool := findTool(current.Tools, call.Name)
	if tool == nil {
		return e.finishError(call, fmt.Sprintf("unknown tool %q", call.Name))
	}
	if tool.Execute == nil {
		return e.finishError(call, fmt.Sprintf("tool %q has no Execute function", call.Name))
	}

	validated, err := llm.ValidateToolArguments(tool.Definition, call)
	if err != nil {
		return e.finishError(call, fmt.Sprintf("invalid tool arguments: %v", err))
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
			return e.finishError(call, reason)
		}
	}

	rawArgs, err := json.Marshal(validated)
	if err != nil {
		return e.finishError(call, fmt.Sprintf("encode tool arguments: %v", err))
	}

	onUpdate := func(partial ToolResult) {
		e.emit(AgentEvent{
			Type:       ToolUpdate,
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Args:       validated,
			Result:     partial,
		})
	}

	result, execErr := tool.Execute(e.ctx, call.ID, rawArgs, onUpdate)
	isError := false
	if execErr != nil {
		isError = true
		result = ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: execErr.Error()}}}
	}

	if e.cfg.AfterToolCall != nil {
		override := e.cfg.AfterToolCall(AfterToolCallCtx{
			AssistantMessage: assistant,
			ToolCall:         call,
			Args:             validated,
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

	return e.finish(call, result, isError)
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
