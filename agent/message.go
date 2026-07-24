package agent

import "github.com/ktsoator/or/llm"

// AgentMessage is any entry that can appear in an agent transcript: a standard
// llm message adapted with FromLLM, or an application's own UI-only message
// type that embeds Custom. UI-only messages take part in history and event
// emission but are filtered out by ConvertToLLM before the model sees them.
type AgentMessage interface {
	isAgentMessage()
}

// FromLLM adapts a standard llm.Message into an AgentMessage. This is the
// common path for user, assistant, and tool-result messages, since the agent
// package cannot add methods to types owned by llm.
func FromLLM(m llm.Message) AgentMessage {
	return llmMessage{Message: m}
}

// ToLLM returns the standard llm.Message wrapped by FromLLM, including through
// an internal queue envelope. It reports false for a custom (UI-only)
// AgentMessage with no LLM projection.
func ToLLM(m AgentMessage) (llm.Message, bool) {
	switch message := m.(type) {
	case llmMessage:
		return message.Message, true
	case queuedMessageEnvelope:
		return ToLLM(message.message)
	default:
		return nil, false
	}
}

// QueueHandleOf returns the queue identity attached to a drained message.
// Ordinary prompts and messages that never passed through a queue have none.
func QueueHandleOf(message AgentMessage) (QueueHandle, bool) {
	queued, ok := message.(queuedMessageEnvelope)
	if !ok {
		return QueueHandle{}, false
	}
	return queued.handle, true
}

// UserMessage builds a user AgentMessage from text and optional images — the
// common case for a multimodal prompt. The text block comes first, followed by
// each image in order. Pass the result to Prompt, Steer, FollowUp, or use it as
// a seed message.
func UserMessage(text string, images ...llm.ImageContent) AgentMessage {
	content := make([]llm.UserContent, 0, 1+len(images))
	content = append(content, &llm.TextContent{Text: text})
	for index := range images {
		image := images[index]
		content = append(content, &image)
	}
	return FromLLM(&llm.UserMessage{Content: content})
}

// llmMessage wraps a standard llm.Message so it satisfies AgentMessage.
type llmMessage struct {
	Message llm.Message
}

func (llmMessage) isAgentMessage() {}

// queuedMessageEnvelope carries queue identity through RunLoop events without
// exposing it to the model or retaining it in the Agent transcript.
type queuedMessageEnvelope struct {
	message AgentMessage
	handle  QueueHandle
}

func (queuedMessageEnvelope) isAgentMessage() {}

func withoutQueueHandle(message AgentMessage) AgentMessage {
	for {
		queued, ok := message.(queuedMessageEnvelope)
		if !ok {
			return message
		}
		message = queued.message
	}
}

// Custom is embedded by an application's own message types so they satisfy
// AgentMessage without referencing the interface's unexported marker.
//
//	type Notification struct {
//		agent.Custom
//		Text string
//	}
type Custom struct{}

func (Custom) isAgentMessage() {}
