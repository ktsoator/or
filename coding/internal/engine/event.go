package engine

import (
	"sync"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type eventBus struct {
	mu        sync.Mutex
	listeners map[int]func(Event)
	nextID    int
}

// EventType identifies a UI-neutral coding-session event. Product adapters
// render these events for their own transport instead of depending on the
// lower-level agent event model.
type EventType string

const (
	RunStarted           EventType = "run_started"
	UserMessageCompleted EventType = "user_message_completed"
	TextDelta            EventType = "text_delta"
	ThinkingDelta        EventType = "thinking_delta"
	ToolInputStarted     EventType = "tool_input_started"
	ToolInputDelta       EventType = "tool_input_delta"
	ToolInputCompleted   EventType = "tool_input_completed"
	ToolStarted          EventType = "tool_started"
	ToolFinished         EventType = "tool_finished"
	MessageCompleted     EventType = "message_completed"
	TurnDiscarded        EventType = "turn_discarded"
	CompactionStarted    EventType = "compaction_started"
	CompactionCompleted  EventType = "compaction_completed"
	CompactionFailed     EventType = "compaction_failed"
	TaskStarted          EventType = "task_started"
	TaskCompleted        EventType = "task_completed"
	RunCompleted         EventType = "run_completed"
)

// Event is the stable event contract exposed by Session. Fields are populated
// according to Type; presentation-specific concerns such as ANSI styling, JSON
// field names, SSE framing, and Markdown rendering stay in product adapters.
type Event struct {
	Type EventType

	// User messages, assistant content, and tool-result media.
	Delta  string
	Text   string
	Images []llm.ImageContent
	Files  []File
	// QueueHandle identifies the queued user message represented by a
	// UserMessageCompleted event. It is zero for an ordinary prompt.
	QueueHandle QueueHandle
	// FinalResponse distinguishes a user-visible completed reply from an
	// assistant message that paused only to call tools.
	FinalResponse bool

	// Tool lifecycle data.
	ToolCallID       string
	ToolName         string
	ToolArgs         any
	ToolContentIndex int
	ToolInputBytes   int
	ToolResult       string
	// ToolOutcome is the source of truth for status, error metadata, and
	// structured product data. ToolResult remains the model-facing text fallback.
	ToolOutcome agent.ToolOutcome

	// BackgroundTask contains the latest lifecycle state for task events.
	BackgroundTask BackgroundTask

	// Usage is one assistant request's consumption on MessageCompleted and the
	// aggregate consumption on RunCompleted. Product adapters may accumulate
	// tool-use requests until FinalResponse to show one total per visible reply.
	Usage llm.Usage
	// ContextUsage is the provider-measured latest context plus its estimated
	// category attribution. It is populated on MessageCompleted.
	ContextUsage ContextUsage

	// Response metadata identifies the exact provider request represented by a
	// MessageCompleted event. It lets product shells build durable, per-model
	// usage reports without inferring the active model from mutable UI state.
	Provider      string
	Model         string
	ResponseModel string
	ResponseID    string
	Timestamp     time.Time
	// Automatic distinguishes context maintenance performed inside an active run
	// from an explicit Compact call. Error is populated on CompactionFailed.
	Automatic bool
	Error     string

	// Run timing is populated on RunStarted and RunCompleted. It measures the
	// full invocation, including model calls, tools, approvals, retries, and any
	// steering or follow-up work consumed before the run ends.
	StartedAt   time.Time
	CompletedAt time.Time
}

// Subscribe registers a listener for UI-neutral coding events and returns a
// function that removes it.
func (s *Session) Subscribe(listener func(Event)) (unsubscribe func()) {
	s.events.mu.Lock()
	if s.events.listeners == nil {
		s.events.listeners = make(map[int]func(Event))
	}
	id := s.events.nextID
	s.events.nextID++
	s.events.listeners[id] = listener
	s.events.mu.Unlock()
	return func() {
		s.events.mu.Lock()
		delete(s.events.listeners, id)
		s.events.mu.Unlock()
	}
}

func (s *Session) dispatchEvent(event Event) {
	s.events.mu.Lock()
	listeners := make([]func(Event), 0, len(s.events.listeners))
	for _, listener := range s.events.listeners {
		listeners = append(listeners, listener)
	}
	s.events.mu.Unlock()
	for _, listener := range listeners {
		listener(event)
	}
}

// projectAgentEvent maps a low-level agent event into the stable coding event
// contract. Agent events without product-visible meaning are omitted.
func projectAgentEvent(ev agent.AgentEvent) (Event, bool) {
	switch ev.Type {
	case agent.AgentStart:
		// Session.run emits one outer RunStarted event. AgentStart can occur again
		// during an application-level retry and must not reset the visible timer.
		return Event{}, false

	case agent.MessageUpdate:
		if ev.LLMEvent == nil {
			return Event{}, false
		}
		switch ev.LLMEvent.Type {
		case llm.EventTextDelta:
			return Event{Type: TextDelta, Delta: ev.LLMEvent.Delta}, true
		case llm.EventThinkingDelta:
			return Event{Type: ThinkingDelta, Delta: ev.LLMEvent.Delta}, true
		case llm.EventToolCallStart:
			return projectToolInputEvent(ToolInputStarted, ev.LLMEvent), true
		case llm.EventToolCallDelta:
			projected := projectToolInputEvent(ToolInputDelta, ev.LLMEvent)
			projected.Delta = ev.LLMEvent.Delta
			projected.ToolInputBytes = len([]byte(ev.LLMEvent.Delta))
			return projected, true
		case llm.EventToolCallEnd:
			return projectToolInputEvent(ToolInputCompleted, ev.LLMEvent), true
		default:
			return Event{}, false
		}

	case agent.ToolStart:
		return Event{
			Type:       ToolStarted,
			ToolCallID: ev.ToolCallID,
			ToolName:   ev.ToolName,
			ToolArgs:   ev.Args,
		}, true

	case agent.ToolEnd:
		return Event{
			Type:        ToolFinished,
			ToolCallID:  ev.ToolCallID,
			ToolName:    ev.ToolName,
			ToolResult:  eventToolResultText(ev.Result),
			Images:      toolResultContentImages(ev.Result.Content),
			ToolOutcome: eventToolOutcome(ev.Result),
		}, true

	case agent.MessageEnd:
		if text, images, files, ok := eventUserMessage(ev.Message); ok {
			projected := Event{
				Type:   UserMessageCompleted,
				Text:   text,
				Images: images,
				Files:  files,
			}
			if handle, queued := agent.QueueHandleOf(ev.Message); queued {
				projected.QueueHandle = QueueHandle{agent: handle}
			}
			return projected, true
		}
		assistant, ok := eventAssistantMessage(ev.Message)
		if !ok {
			return Event{}, false
		}
		return Event{
			Type: MessageCompleted,
			Text: displayAssistantText(assistant),
			FinalResponse: assistant.StopReason != llm.StopReasonToolUse &&
				assistant.StopReason != llm.StopReasonError &&
				assistant.StopReason != llm.StopReasonAborted,
			Usage:         assistant.Usage,
			Provider:      assistant.Provider,
			Model:         assistant.Model,
			ResponseModel: assistant.ResponseModel,
			ResponseID:    assistant.ResponseID,
			Timestamp:     time.UnixMilli(assistant.Timestamp).UTC(),
			CompletedAt:   time.Now().UTC(),
		}, true

	case agent.AgentEnd:
		// Session.run emits RunCompleted after retries and persistence have finished.
		return Event{}, false

	default:
		return Event{}, false
	}
}

func projectToolInputEvent(eventType EventType, event *llm.Event) Event {
	projected := Event{
		Type:             eventType,
		ToolContentIndex: event.ContentIndex,
	}
	if event.ToolCall != nil {
		projected.ToolCallID = event.ToolCall.ID
		projected.ToolName = event.ToolCall.Name
		if eventType == ToolInputCompleted {
			projected.ToolArgs = event.ToolCall.Arguments
		}
	}
	return projected
}

func eventUserMessage(
	message agent.AgentMessage,
) (string, []llm.ImageContent, []File, bool) {
	llmMessage, ok := agent.ToLLM(message)
	if !ok {
		return "", nil, nil, false
	}
	user, ok := llmMessage.(*llm.UserMessage)
	if !ok {
		return "", nil, nil, false
	}
	text, images, files := userMessageContent(user)
	return text, images, files, true
}

func addUsage(total *llm.Usage, usage llm.Usage) {
	total.Input += usage.Input
	total.InputUnknown = total.InputUnknown || usage.InputUnknown
	total.Output += usage.Output
	total.CacheRead += usage.CacheRead
	total.CacheWrite += usage.CacheWrite
	tokens := usage.TotalTokens
	if tokens == 0 {
		tokens = usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	}
	total.TotalTokens += tokens
	total.Cost.Input += usage.Cost.Input
	total.Cost.Output += usage.Cost.Output
	total.Cost.CacheRead += usage.Cost.CacheRead
	total.Cost.CacheWrite += usage.Cost.CacheWrite
	total.Cost.Total += usage.Cost.Total
}

func hasUsage(usage llm.Usage) bool {
	return usage.InputUnknown || usage.Input != 0 || usage.Output != 0 || usage.CacheRead != 0 ||
		usage.CacheWrite != 0 || usage.TotalTokens != 0 || usage.Cost.Total != 0
}

// eventAssistantText returns displayable assistant text. Failed messages retain
// their terminal state; aborted messages keep only content that actually
// streamed before the user stopped the run.
func eventAssistantText(message agent.AgentMessage) (string, bool) {
	assistant, ok := eventAssistantMessage(message)
	if !ok {
		return "", false
	}
	return displayAssistantText(assistant), true
}

func eventAssistantMessage(message agent.AgentMessage) (*llm.AssistantMessage, bool) {
	llmMessage, ok := agent.ToLLM(message)
	if !ok {
		return nil, false
	}
	assistant, ok := llmMessage.(*llm.AssistantMessage)
	if !ok || assistant == nil {
		return nil, false
	}
	return assistant, true
}

func displayAssistantText(assistant *llm.AssistantMessage) string {
	if assistant.StopReason == llm.StopReasonAborted {
		return assistant.Text()
	}
	if assistant.StopReason == llm.StopReasonError {
		if assistant.ErrorMessage != "" {
			return "[" + string(assistant.StopReason) + "] " + assistant.ErrorMessage
		}
		return "[" + string(assistant.StopReason) + "]"
	}
	return assistant.Text()
}

// eventToolResultText extracts the textual projection of a tool result.
func eventToolResultText(result agent.ToolResult) string {
	return toolResultContentText(result.Content)
}

func eventToolOutcome(result agent.ToolResult) agent.ToolOutcome {
	return result.Outcome
}
