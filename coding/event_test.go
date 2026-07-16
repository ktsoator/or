package coding

import (
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestProjectAgentEventTextDelta(t *testing.T) {
	llmEvent := &llm.Event{Type: llm.EventTextDelta, Delta: "hello"}
	event, ok := projectAgentEvent(agent.AgentEvent{
		Type:     agent.MessageUpdate,
		LLMEvent: llmEvent,
	})
	if !ok {
		t.Fatal("text delta was not projected")
	}
	if event.Type != TextDelta || event.Delta != "hello" {
		t.Fatalf("event = %+v", event)
	}
}

func TestProjectAgentEventToolResult(t *testing.T) {
	result := agent.ToolResult{Content: []llm.ToolResultContent{
		&llm.TextContent{Text: "first"},
		&llm.TextContent{Text: "second"},
	}}
	event, ok := projectAgentEvent(agent.AgentEvent{
		Type:       agent.ToolEnd,
		ToolCallID: "call-7",
		ToolName:   "read",
		Result:     result,
		IsError:    true,
	})
	if !ok {
		t.Fatal("tool result was not projected")
	}
	if event.Type != ToolFinished || event.ToolCallID != "call-7" || event.ToolName != "read" || event.ToolResult != "first\nsecond" || !event.IsError {
		t.Fatalf("event = %+v", event)
	}
}

func TestProjectAgentEventCompletedAssistant(t *testing.T) {
	message := &llm.AssistantMessage{
		Content:    []llm.AssistantContent{&llm.TextContent{Text: "done"}},
		StopReason: llm.StopReasonStop,
	}
	event, ok := projectAgentEvent(agent.AgentEvent{
		Type:    agent.MessageEnd,
		Message: agent.FromLLM(message),
	})
	if !ok {
		t.Fatal("assistant message was not projected")
	}
	if event.Type != MessageCompleted || event.Text != "done" {
		t.Fatalf("event = %+v", event)
	}
}

func TestProjectAgentEventOmitsUserMessage(t *testing.T) {
	if _, ok := projectAgentEvent(agent.AgentEvent{
		Type:    agent.MessageEnd,
		Message: agent.UserMessage("hello"),
	}); ok {
		t.Fatal("user message should not be projected as assistant output")
	}
}
