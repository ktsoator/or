package compaction

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestPrepareCutsAtUserTurnWithoutSplittingToolPair(t *testing.T) {
	call := &llm.AssistantMessage{
		Content: []llm.AssistantContent{&llm.ToolCall{
			ID: "call-1", Name: "read", Arguments: map[string]any{"path": "old.go"},
		}},
		StopReason: llm.StopReasonToolUse,
	}
	result := &llm.ToolResultMessage{
		ToolCallID: "call-1", ToolName: "read",
		Content: []llm.ToolResultContent{&llm.TextContent{Text: strings.Repeat("result ", 40)}},
	}
	messages := []agent.AgentMessage{
		agent.UserMessage(strings.Repeat("old request ", 30)),
		agent.FromLLM(call),
		agent.FromLLM(result),
		agent.FromLLM(&llm.AssistantMessage{Content: []llm.AssistantContent{&llm.TextContent{Text: "old done"}}, StopReason: llm.StopReasonStop}),
		agent.UserMessage(strings.Repeat("new request ", 30)),
		agent.FromLLM(&llm.AssistantMessage{Content: []llm.AssistantContent{&llm.TextContent{Text: strings.Repeat("new answer ", 30)}}, StopReason: llm.StopReasonStop}),
	}
	entries := makeEntries(messages)

	prepared, err := Prepare(entries, 20)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.FirstKeptID != entries[4].ID {
		t.Fatalf("first kept = %s, want second user %s", prepared.FirstKeptID, entries[4].ID)
	}
	if len(prepared.Messages) != 4 {
		t.Fatalf("summarized messages = %d, want complete first turn of 4", len(prepared.Messages))
	}
	if message, _ := agent.ToLLM(prepared.Messages[2]); message.(*llm.ToolResultMessage).ToolCallID != "call-1" {
		t.Fatal("tool result was not retained with its tool call in summarized prefix")
	}
}

func makeEntries(messages []agent.AgentMessage) []transcript.Entry {
	entries := make([]transcript.Entry, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, transcript.NewMessage(message))
	}
	return entries
}
