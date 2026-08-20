package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/ktsoator/or/llm"
)

// errBusy is returned by Prompt when a run is already in progress.
var errBusy = errors.New("agent: a prompt is already in progress")

// Prompt starts a run from a text string, a single AgentMessage, or a slice of
// them, and blocks until the run completes. It appends the run's messages to the
// transcript and returns an error if the run ended in failure or cancellation.
// Calling Prompt while a run is in progress returns an error.
func (a *Agent) Prompt(ctx context.Context, input any) error {
	prompts, err := toPrompts(input)
	if err != nil {
		return err
	}
	return a.run(ctx, prompts, false)
}

// Continue resumes a run from the current transcript without adding a new
// message, blocking until it completes. Use it to retry or to respond after
// messages were appended out of band.
//
// The transcript must be non-empty. A provider needs a user or tool result as
// the latest message, so when the transcript ends with an assistant message,
// Continue falls back to queued messages: it drains the steering queue first,
// then the follow-up queue, and runs whatever it finds as the next prompt. It
// returns an error only when the last message is an assistant and both queues
// are empty.
func (a *Agent) Continue(ctx context.Context) error {
	a.mu.Lock()
	if a.isStreaming {
		a.mu.Unlock()
		return errBusy
	}
	count := len(a.messages)
	lastIsAssistant := false
	if count > 0 {
		_, lastIsAssistant = assistantMessage(a.messages[count-1])
	}
	a.mu.Unlock()

	if count == 0 {
		return errors.New("agent: cannot continue an empty transcript")
	}
	if lastIsAssistant {
		// Drained steering messages already become the run's prompt, so the
		// loop's first steering poll is skipped to avoid injecting them twice.
		if steering := a.steering.drain(); len(steering) > 0 {
			return a.run(ctx, steering, true)
		}
		if followUp := a.followUp.drain(); len(followUp) > 0 {
			return a.run(ctx, followUp, false)
		}
		return errors.New("agent: cannot continue from an assistant message")
	}
	return a.run(ctx, nil, false)
}

// run drives one RunLoop invocation from prompts and the current state, then
// commits the appended messages to the transcript. skipInitialSteering omits the
// loop's first steering poll, used when the prompts were themselves drained from
// the steering queue.
func (a *Agent) run(ctx context.Context, prompts []AgentMessage, skipInitialSteering bool) error {
	if ctx == nil {
		ctx = context.Background()
	}

	a.mu.Lock()
	if a.isStreaming {
		a.mu.Unlock()
		return errBusy
	}
	a.isStreaming = true
	a.errorMessage = ""
	a.streamingMessage = nil
	a.pendingToolCalls = nil
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	base := Context{
		SystemPrompt: a.systemPrompt,
		Messages:     append([]AgentMessage(nil), a.messages...),
		Tools:        append([]AgentTool(nil), a.tools...),
	}
	cfg := a.loopConfigLocked(skipInitialSteering)
	a.mu.Unlock()

	defer cancel()

	var appended []AgentMessage
	for event := range RunLoop(runCtx, prompts, base, cfg) {
		if event.Type == AgentEnd {
			appended = event.Messages
		}
		a.reduce(event)
		a.dispatch(event)
	}

	errText := lastAssistantError(appended)
	runContextErr := runCtx.Err()

	a.mu.Lock()
	a.isStreaming = false
	a.cancel = nil
	a.errorMessage = errText
	a.streamingMessage = nil
	a.pendingToolCalls = nil
	a.mu.Unlock()

	if errText != "" {
		// Preserve cancellation identity so product adapters can distinguish an
		// intentional abort from an actual model or transport failure.
		if runContextErr != nil {
			return runContextErr
		}
		return errors.New(errText)
	}
	return nil
}

// Abort cancels the current run, if any.
func (a *Agent) Abort() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// loopConfigLocked builds the LoopConfig for one run. The caller holds a.mu.
// When skipInitialSteering is set, the first steering poll returns nothing, so
// messages already drained into the run's prompt are not injected a second time.
func (a *Agent) loopConfigLocked(skipInitialSteering bool) LoopConfig {
	getSteering := a.steering.drain
	if skipInitialSteering {
		skipped := false
		getSteering = func() []AgentMessage {
			if !skipped {
				skipped = true
				return nil
			}
			return a.steering.drain()
		}
	}
	streamOptions := a.streamOptions
	streamOptions.Reasoning = a.thinkingLevel
	return LoopConfig{
		Model:               a.model,
		StreamOptions:       streamOptions,
		StreamFn:            a.streamFn,
		GetAPIKey:           a.getAPIKey,
		ConvertToLLM:        a.convertToLLM,
		TransformContext:    a.transformContext,
		ToolExecution:       a.toolExecution,
		BeforeToolCall:      a.beforeToolCall,
		AfterToolCall:       a.afterToolCall,
		ShouldStopAfterStep: a.shouldStopAfterStep,
		PrepareNextStep:     a.prepareNextStep,
		GetSteeringMessages: getSteering,
		GetFollowUpMessages: a.followUp.drain,
	}
}

// toPrompts normalizes Prompt input into messages.
func toPrompts(input any) ([]AgentMessage, error) {
	switch value := input.(type) {
	case string:
		return []AgentMessage{FromLLM(&llm.UserMessage{
			Content: []llm.UserContent{&llm.TextContent{Text: value}},
		})}, nil
	case AgentMessage:
		return []AgentMessage{value}, nil
	case []AgentMessage:
		if len(value) == 0 {
			return nil, errors.New("agent: prompt input is empty")
		}
		return append([]AgentMessage(nil), value...), nil
	case nil:
		return nil, errors.New("agent: prompt input is nil")
	default:
		return nil, fmt.Errorf("agent: unsupported prompt input type %T", input)
	}
}

// lastAssistantError returns the error text of the run's final assistant step
// when it failed or was aborted, or "" otherwise.
func lastAssistantError(messages []AgentMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		assistant, ok := assistantMessage(messages[index])
		if !ok {
			continue
		}
		if assistant.StopReason == llm.StopReasonError || assistant.StopReason == llm.StopReasonAborted {
			if assistant.ErrorMessage != "" {
				return assistant.ErrorMessage
			}
			return string(assistant.StopReason)
		}
		return ""
	}
	return ""
}

// assistantMessage unwraps an AgentMessage into an llm assistant message.
func assistantMessage(message AgentMessage) (*llm.AssistantMessage, bool) {
	projected, ok := ToLLM(message)
	if !ok {
		return nil, false
	}
	assistant, ok := projected.(*llm.AssistantMessage)
	return assistant, ok
}
