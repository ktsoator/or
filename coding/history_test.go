package coding

import (
	"reflect"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestProjectHistoryPreservesDisplayOrder(t *testing.T) {
	messages := []agent.AgentMessage{
		agent.FromLLM(&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: "inspect the project"},
		}}),
		agent.FromLLM(&llm.AssistantMessage{
			Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "locating files"},
				&llm.TextContent{Text: "I'll inspect it."},
				&llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{"path": "main.go"}},
			},
			StopReason: llm.StopReasonToolUse,
		}),
		agent.FromLLM(&llm.ToolResultMessage{
			ToolCallID: "call-1",
			ToolName:   "read",
			Content:    []llm.ToolResultContent{&llm.TextContent{Text: "package main"}},
		}),
		agent.FromLLM(&llm.AssistantMessage{
			Content:    []llm.AssistantContent{&llm.TextContent{Text: "Done."}},
			StopReason: llm.StopReasonStop,
		}),
	}

	got := projectHistory(messages)
	want := []HistoryItem{
		{Type: HistoryUser, Text: "inspect the project"},
		{Type: HistoryThinking, Text: "locating files"},
		{Type: HistoryAssistant, Text: "I'll inspect it."},
		{Type: HistoryToolCall, ToolCallID: "call-1", ToolName: "read", ToolArgs: map[string]any{"path": "main.go"}},
		{Type: HistoryToolResult, ToolCallID: "call-1", ToolName: "read", ToolResult: "package main"},
		{Type: HistoryAssistant, Text: "Done."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history = %#v\nwant    = %#v", got, want)
	}
}

func TestProjectHistoryRendersTerminalAssistantError(t *testing.T) {
	got := projectHistory([]agent.AgentMessage{agent.FromLLM(&llm.AssistantMessage{
		StopReason:   llm.StopReasonError,
		ErrorMessage: "provider unavailable",
	})})
	want := []HistoryItem{{Type: HistoryAssistant, Text: "[error] provider unavailable"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history = %#v, want %#v", got, want)
	}
}
