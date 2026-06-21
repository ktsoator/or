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
