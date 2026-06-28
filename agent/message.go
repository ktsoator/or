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

// ToLLM returns the standard llm.Message wrapped by FromLLM, reporting false for
// a custom (UI-only) AgentMessage that has no llm projection. It is the inverse
// of FromLLM, intended for use inside a custom ConvertToLLM that needs to pass
// the adapted messages through while projecting its own message types.
func ToLLM(m AgentMessage) (llm.Message, bool) {
	wrapped, ok := m.(llmMessage)
	if !ok {
		return nil, false
	}
	return wrapped.Message, true
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

// Custom is embedded by an application's own message types so they satisfy
// AgentMessage without referencing the interface's unexported marker.
//
//	type Notification struct {
//		agent.Custom
//		Text string
//	}
type Custom struct{}

func (Custom) isAgentMessage() {}
