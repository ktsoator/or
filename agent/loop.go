package agent

import (
	"context"

	"github.com/ktsoator/or/llm"
)

// StreamFn reaches a model for one turn. It has the same shape as llm.Stream,
// which is the default when LoopConfig.StreamFn is nil.
//
// Like llm.Stream, it must not fail by panicking: a setup failure is returned as
// an error, and a mid-stream failure arrives on the channel as an EventError
// carrying an AssistantMessage whose StopReason is error or aborted.
type StreamFn func(ctx context.Context, model llm.Model, input llm.Context, options llm.StreamOptions) (<-chan llm.Event, error)

// RunLoop drives a complete tool-call loop and returns a channel of events. The
// final AgentEnd event carries the messages the run appended to the transcript.
//
// prompts are the new messages that start the run; base is the existing context
// they extend. The loop streams an assistant turn, executes any tool calls,
// appends results, then consults PrepareNextTurn and ShouldStopAfterTurn before
// continuing. When no tool calls and no steering messages remain, it polls
// GetFollowUpMessages; if none arrive, the run ends.
//
// The caller must drain the returned channel until it closes.
func RunLoop(ctx context.Context, prompts []AgentMessage, base Context, cfg LoopConfig) <-chan AgentEvent {
	events := make(chan AgentEvent)

	e := &engine{
		ctx:      ctx,
		cfg:      cfg,
		streamFn: cfg.StreamFn,
		convert:  cfg.ConvertToLLM,
		emit:     func(event AgentEvent) { events <- event },
	}
	if e.streamFn == nil {
		e.streamFn = llm.Stream
	}
	if e.convert == nil {
		e.convert = defaultConvertToLLM
	}

	go func() {
		defer close(events)
		e.run(prompts, base)
	}()

	return events
}

// engine holds the mutable state of one RunLoop invocation. cfg changes across
// turns when PrepareNextTurn replaces the model or thinking level.
type engine struct {
	ctx      context.Context
	cfg      LoopConfig
	streamFn StreamFn
	convert  func([]AgentMessage) []llm.Message
	emit     func(AgentEvent)
}

func (e *engine) run(prompts []AgentMessage, base Context) {
	newMessages := append([]AgentMessage(nil), prompts...)
	current := base
	current.Messages = append(append([]AgentMessage(nil), base.Messages...), prompts...)

	e.emit(AgentEvent{Type: AgentStart})
	e.emit(AgentEvent{Type: TurnStart})
	for _, prompt := range prompts {
		e.emit(AgentEvent{Type: MessageStart, Message: prompt})
		e.emit(AgentEvent{Type: MessageEnd, Message: prompt})
	}

	firstTurn := true
	var pending []AgentMessage
	if e.cfg.GetSteeringMessages != nil {
		pending = e.cfg.GetSteeringMessages()
	}

	for {
		hasMoreToolCalls := true

		for hasMoreToolCalls || len(pending) > 0 {
			if firstTurn {
				firstTurn = false
			} else {
				e.emit(AgentEvent{Type: TurnStart})
			}

			for _, message := range pending {
				e.emit(AgentEvent{Type: MessageStart, Message: message})
				e.emit(AgentEvent{Type: MessageEnd, Message: message})
				current.Messages = append(current.Messages, message)
				newMessages = append(newMessages, message)
			}
			pending = nil

			message := e.streamAssistant(current)
			assistant := FromLLM(&message)
			current.Messages = append(current.Messages, assistant)
			newMessages = append(newMessages, assistant)

			if message.StopReason == llm.StopReasonError || message.StopReason == llm.StopReasonAborted {
				e.emit(AgentEvent{Type: TurnEnd, Message: assistant})
				e.emit(AgentEvent{Type: AgentEnd, Messages: newMessages})
				return
			}

			toolCalls := assistantToolCalls(message)
			var toolResults []llm.ToolResultMessage
			hasMoreToolCalls = false
			if len(toolCalls) > 0 {
				results, terminate := e.executeToolCalls(current, message, toolCalls)
				toolResults = results
				hasMoreToolCalls = !terminate
				for index := range results {
					current.Messages = append(current.Messages, FromLLM(&results[index]))
					newMessages = append(newMessages, FromLLM(&results[index]))
				}
			}

			e.emit(AgentEvent{Type: TurnEnd, Message: FromLLM(&message), ToolResults: toolResults})

			turn := TurnCtx{
				Message:     message,
				ToolResults: toolResults,
				Context:     current,
				NewMessages: newMessages,
			}
			if e.cfg.PrepareNextTurn != nil {
				if update := e.cfg.PrepareNextTurn(turn); update != nil {
					if update.Context != nil {
						current = *update.Context
					}
					if update.Model != nil {
						e.cfg.Model = *update.Model
					}
					if update.ThinkingLevel != nil {
						e.cfg.StreamOptions.Reasoning = *update.ThinkingLevel
					}
				}
			}

			if e.cfg.ShouldStopAfterTurn != nil && e.cfg.ShouldStopAfterTurn(TurnCtx{
				Message:     message,
				ToolResults: toolResults,
				Context:     current,
				NewMessages: newMessages,
			}) {
				e.emit(AgentEvent{Type: AgentEnd, Messages: newMessages})
				return
			}

			if e.cfg.GetSteeringMessages != nil {
				pending = e.cfg.GetSteeringMessages()
			}
		}

		var followUp []AgentMessage
		if e.cfg.GetFollowUpMessages != nil {
			followUp = e.cfg.GetFollowUpMessages()
		}
		if len(followUp) > 0 {
			pending = followUp
			continue
		}
		break
	}

	e.emit(AgentEvent{Type: AgentEnd, Messages: newMessages})
}

// streamAssistant projects the transcript, runs one model request, emits message
// lifecycle events, and returns the final assistant message. A setup failure or
// a stream that closes without a terminal event is reported as an error message
// rather than a Go error, keeping the loop's "errors travel as messages"
// contract.
func (e *engine) streamAssistant(current Context) llm.AssistantMessage {
	messages := current.Messages
	if e.cfg.TransformContext != nil {
		messages = e.cfg.TransformContext(messages)
	}

	input := llm.Context{
		SystemPrompt: current.SystemPrompt,
		Messages:     e.convert(messages),
		Tools:        toolDefinitions(current.Tools),
	}

	stream, err := e.streamFn(e.ctx, e.cfg.Model, input, e.cfg.StreamOptions)
	if err != nil {
		return e.emitErrorMessage(err.Error())
	}

	started := false
	var final *llm.AssistantMessage
	for event := range stream {
		switch event.Type {
		case llm.EventStart:
			if event.Partial != nil {
				started = true
				e.emit(AgentEvent{Type: MessageStart, Message: FromLLM(event.Partial)})
			}
		case llm.EventDone, llm.EventError:
			final = event.Message
		default:
			if event.Partial != nil {
				eventCopy := event
				e.emit(AgentEvent{
					Type:     MessageUpdate,
					Message:  FromLLM(event.Partial),
					LLMEvent: &eventCopy,
				})
			}
		}
	}

	if final == nil {
		return e.emitErrorMessage("stream closed without a final message")
	}
	if !started {
		e.emit(AgentEvent{Type: MessageStart, Message: FromLLM(final)})
	}
	e.emit(AgentEvent{Type: MessageEnd, Message: FromLLM(final)})
	return *final
}

// emitErrorMessage builds an error assistant message for the current model,
// emits its lifecycle events, and returns it.
func (e *engine) emitErrorMessage(text string) llm.AssistantMessage {
	message := llm.AssistantMessage{
		Protocol:     e.cfg.Model.Protocol,
		Provider:     e.cfg.Model.Provider,
		Model:        e.cfg.Model.ID,
		StopReason:   llm.StopReasonError,
		ErrorMessage: text,
	}
	e.emit(AgentEvent{Type: MessageStart, Message: FromLLM(&message)})
	e.emit(AgentEvent{Type: MessageEnd, Message: FromLLM(&message)})
	return message
}
