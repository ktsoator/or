package agent

import "github.com/ktsoator/or/llm"

// Snapshot returns a read-only view of the agent's current state.
func (a *Agent) Snapshot() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return State{
		SystemPrompt:     a.systemPrompt,
		Model:            a.model,
		ThinkingLevel:    a.thinkingLevel,
		Tools:            append([]AgentTool(nil), a.tools...),
		Messages:         append([]AgentMessage(nil), a.messages...),
		IsStreaming:      a.isStreaming,
		StreamingMessage: a.streamingMessage,
		PendingToolCalls: append([]string(nil), a.pendingToolCalls...),
		ErrorMessage:     a.errorMessage,
	}
}

// The setters below reconfigure the agent between runs. Each takes effect on
// the next run; it does not change a run already in progress, which captured its
// configuration when it started. All are safe to call concurrently.

// SetSystemPrompt replaces the system prompt used on the next run.
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	a.systemPrompt = prompt
	a.mu.Unlock()
}

// SetModel replaces the model used on the next run. To switch models within a
// single run, use PrepareNextStep instead.
func (a *Agent) SetModel(model llm.Model) {
	a.mu.Lock()
	a.model = model
	a.mu.Unlock()
}

// SetThinkingLevel replaces the reasoning level used on the next run.
func (a *Agent) SetThinkingLevel(level llm.ModelThinkingLevel) {
	a.mu.Lock()
	a.thinkingLevel = level
	a.mu.Unlock()
}

// SetTools replaces the tools available on the next run. The slice is copied.
func (a *Agent) SetTools(tools []AgentTool) {
	a.mu.Lock()
	a.tools = append([]AgentTool(nil), tools...)
	a.mu.Unlock()
}

// SetToolExecution sets the default tool execution mode for the next run.
func (a *Agent) SetToolExecution(mode ExecutionMode) {
	a.mu.Lock()
	a.toolExecution = mode
	a.mu.Unlock()
}

// SetMessages replaces the transcript, for rewriting history out of band — for
// example to install a compacted transcript. The slice is copied. It is meant to
// be called when the agent is idle.
func (a *Agent) SetMessages(messages []AgentMessage) {
	a.mu.Lock()
	a.messages = append([]AgentMessage(nil), messages...)
	a.mu.Unlock()
}

// Reset clears the transcript, the last error, and both queues, keeping the
// configuration (model, tools, system prompt, hooks). It is meant to be called
// when the agent is idle.
func (a *Agent) Reset() {
	a.steering.clear()
	a.followUp.clear()
	a.mu.Lock()
	a.messages = nil
	a.errorMessage = ""
	a.streamingMessage = nil
	a.pendingToolCalls = nil
	a.mu.Unlock()
}

// reduce folds one run event into the agent's live state so a concurrent
// Snapshot reflects progress mid-run: the transcript grows as messages
// complete, StreamingMessage tracks the in-flight response, and
// PendingToolCalls tracks executing tool calls. It runs in event order, before
// dispatch, so listeners observe the updated state.
func (a *Agent) reduce(event AgentEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch event.Type {
	case MessageStart, MessageUpdate:
		a.streamingMessage = withoutQueueHandle(event.Message)
	case MessageEnd:
		a.streamingMessage = nil
		a.messages = append(a.messages, withoutQueueHandle(event.Message))
	case ToolStart:
		a.pendingToolCalls = append(a.pendingToolCalls, event.ToolCallID)
	case ToolEnd:
		a.pendingToolCalls = removeID(a.pendingToolCalls, event.ToolCallID)
	case AgentEnd:
		a.streamingMessage = nil
	}
}

// removeID returns ids with the first occurrence of id removed, preserving
// order. It is called under a.mu; the returned slice may share backing with the
// input, which is safe because Snapshot copies before exposing it.
func removeID(ids []string, id string) []string {
	for index, existing := range ids {
		if existing == id {
			return append(ids[:index], ids[index+1:]...)
		}
	}
	return ids
}
