package agent

import (
	"context"
	"fmt"

	"github.com/ktsoator/or/internal/llm"
)

// loopConfig is the immutable per-run mechanism configuration.
type loopConfig struct {
	client       *llm.Client
	model        llm.Model
	options      llm.StreamOptions
	systemPrompt string
	tools        map[string]Tool
	definitions  []llm.ToolDefinition
	maxTurns     int
}

// loopHooks are the extension points the loop calls at safe points. The Agent
// wires these to its message queues; the loop itself knows nothing about queues.
type loopHooks struct {
	// getSteering returns messages to inject after a turn's tools finish.
	getSteering func() []llm.Message
	// getFollowUp returns messages to continue with after the agent would stop.
	getFollowUp func() []llm.Message
}

// loop is the pure agent mechanism: stream a turn, execute tool calls, feed the
// results back, and repeat. Steering and follow-up messages enter only through
// the hooks, which keeps the loop independent of how those messages are stored.
type loop struct {
	cfg   loopConfig
	hooks loopHooks
}

// run drives the loop over transcript, appending new messages to it. prompts are
// the initial messages for this run (nil for a continuation). emit returns false
// when the consumer is gone (ctx cancelled); the loop then stops.
func (l *loop) run(ctx context.Context, transcript *[]llm.Message, prompts []llm.Message, emit func(Event) bool) {
	newMessages := append([]llm.Message(nil), prompts...)
	*transcript = append(*transcript, prompts...)

	if !emit(Event{Type: EventAgentStart}) || !emit(Event{Type: EventTurnStart}) {
		return
	}
	for _, prompt := range prompts {
		if !emit(Event{Type: EventMessageStart, Message: prompt}) || !emit(Event{Type: EventMessageEnd, Message: prompt}) {
			return
		}
	}

	pending := l.hooks.getSteering()
	firstTurn := true
	turns := 0

	for {
		hasMoreToolCalls := true
		for hasMoreToolCalls || len(pending) > 0 {
			if turns >= l.cfg.maxTurns {
				emit(Event{Type: EventAgentEnd, Messages: newMessages})
				return
			}
			turns++

			if firstTurn {
				firstTurn = false
			} else if !emit(Event{Type: EventTurnStart}) {
				return
			}

			for _, message := range pending {
				if !emit(Event{Type: EventMessageStart, Message: message}) || !emit(Event{Type: EventMessageEnd, Message: message}) {
					return
				}
				*transcript = append(*transcript, message)
				newMessages = append(newMessages, message)
			}
			pending = nil

			message, ok := l.streamTurn(ctx, *transcript, emit)
			if !ok {
				return
			}
			*transcript = append(*transcript, &message)
			newMessages = append(newMessages, &message)

			if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
				emit(Event{Type: EventTurnEnd, Message: &message})
				emit(Event{Type: EventAgentEnd, Messages: newMessages})
				return
			}

			toolCalls := assistantToolCalls(&message)
			var toolResults []llm.Message
			hasMoreToolCalls = false
			if len(toolCalls) > 0 {
				results, terminate, ok := l.executeToolCalls(ctx, toolCalls, emit)
				if !ok {
					return
				}
				toolResults = results
				hasMoreToolCalls = !terminate
				for _, result := range results {
					*transcript = append(*transcript, result)
					newMessages = append(newMessages, result)
				}
			}

			if !emit(Event{Type: EventTurnEnd, Message: &message, ToolResults: toolResults}) {
				return
			}

			pending = l.hooks.getSteering()
		}

		// The agent would stop here. Continue if follow-up messages are queued.
		if followUp := l.hooks.getFollowUp(); len(followUp) > 0 {
			pending = followUp
			continue
		}
		break
	}

	emit(Event{Type: EventAgentEnd, Messages: newMessages})
}

// streamTurn streams one assistant turn, forwarding llm stream events as
// message_update events, and returns the final assistant message. The bool is
// false when ctx was cancelled while emitting (the caller should stop).
func (l *loop) streamTurn(ctx context.Context, transcript []llm.Message, emit func(Event) bool) (llm.AssistantMessage, bool) {
	input := llm.Context{
		SystemPrompt: l.cfg.systemPrompt,
		Messages:     transcript,
		Tools:        l.cfg.definitions,
	}

	stream, err := l.cfg.client.Stream(ctx, l.cfg.model, input, l.cfg.options)
	if err != nil {
		message := llm.NewAssistantMessage(l.cfg.model)
		message.StopReason = llm.StopReasonError
		message.ErrorMessage = err.Error()
		ok := emit(Event{Type: EventMessageStart, Message: &message}) &&
			emit(Event{Type: EventMessageEnd, Message: &message})
		return message, ok
	}

	var final *llm.AssistantMessage
	started := false
	for event := range stream {
		switch event.Type {
		case llm.EventStart:
			if event.Partial != nil {
				started = true
				if !emit(Event{Type: EventMessageStart, Message: event.Partial}) {
					drain(stream)
					return llm.AssistantMessage{}, false
				}
			}
		case llm.EventDone, llm.EventError:
			final = event.Message
		default:
			llmEvent := event
			if !emit(Event{Type: EventMessageUpdate, Message: event.Partial, LLMEvent: &llmEvent}) {
				drain(stream)
				return llm.AssistantMessage{}, false
			}
		}
	}

	if final == nil {
		message := llm.NewAssistantMessage(l.cfg.model)
		message.StopReason = llm.StopReasonError
		message.ErrorMessage = "stream ended without a final message"
		final = &message
	}
	if !started {
		if !emit(Event{Type: EventMessageStart, Message: final}) {
			return *final, false
		}
	}
	if !emit(Event{Type: EventMessageEnd, Message: final}) {
		return *final, false
	}
	return *final, true
}

// executeToolCalls runs each tool call sequentially, emitting tool events and
// collecting tool result messages. terminate is true when every result asks to
// stop. The final bool is false when ctx was cancelled while emitting.
func (l *loop) executeToolCalls(ctx context.Context, toolCalls []*llm.ToolCall, emit func(Event) bool) ([]llm.Message, bool, bool) {
	results := make([]llm.Message, 0, len(toolCalls))
	terminate := true

	for _, call := range toolCalls {
		if !emit(Event{Type: EventToolStart, ToolCallID: call.ID, ToolName: call.Name, Args: call.Arguments}) {
			return nil, false, false
		}

		result, isError := l.runTool(ctx, call)

		if !emit(Event{Type: EventToolEnd, ToolCallID: call.ID, ToolName: call.Name, Result: &result, IsError: isError}) {
			return nil, false, false
		}

		results = append(results, &llm.ToolResultMessage{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    result.Content,
			IsError:    isError,
		})
		if !result.Terminate {
			terminate = false
		}
		if ctx.Err() != nil {
			break
		}
	}

	return results, terminate, true
}

func (l *loop) runTool(ctx context.Context, call *llm.ToolCall) (Result, bool) {
	tool, ok := l.cfg.tools[call.Name]
	if !ok {
		return errorResult(fmt.Sprintf("unknown tool: %q", call.Name)), true
	}
	arguments, err := llm.ValidateToolArguments(tool.Definition(), *call)
	if err != nil {
		return errorResult(err.Error()), true
	}
	result, err := tool.Execute(ctx, arguments)
	if err != nil {
		return errorResult(err.Error()), true
	}
	return result, false
}

func errorResult(message string) Result {
	return Result{Content: []llm.ToolResultContent{&llm.TextContent{Text: message}}}
}

func assistantToolCalls(message *llm.AssistantMessage) []*llm.ToolCall {
	var calls []*llm.ToolCall
	for _, content := range message.Content {
		if call, ok := content.(*llm.ToolCall); ok {
			calls = append(calls, call)
		}
	}
	return calls
}

// drain consumes the remaining llm stream so its producer goroutine can exit
// after the agent stops reading early.
func drain(stream <-chan llm.Event) {
	for range stream {
	}
}
