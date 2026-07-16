package web

import (
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/coding"
)

func TestProjectEventKeepsToolCallID(t *testing.T) {
	tests := []coding.Event{
		{Type: coding.ToolStarted, ToolCallID: "call-42", ToolName: "read_file"},
		{Type: coding.ToolFinished, ToolCallID: "call-42", ToolName: "read_file", ToolResult: "done"},
	}

	for _, event := range tests {
		data, ok := ProjectEvent(event)
		if !ok {
			t.Fatalf("event %q was not projected", event.Type)
		}
		var wire wireEvent
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.ID != "call-42" {
			t.Fatalf("event %q ID = %q", event.Type, wire.ID)
		}
	}
}

func TestProjectHistoryUsesRenderableEventShapes(t *testing.T) {
	got := ProjectHistory([]coding.HistoryItem{
		{Type: coding.HistoryUser, Text: "hello"},
		{Type: coding.HistoryThinking, Text: "checking"},
		{Type: coding.HistoryToolCall, ToolCallID: "call-1", ToolName: "read", ToolArgs: map[string]any{"path": "main.go"}},
		{Type: coding.HistoryToolResult, ToolCallID: "call-1", ToolName: "read", ToolResult: "done"},
		{Type: coding.HistoryAssistant, Text: "finished"},
	})

	if len(got) != 5 {
		t.Fatalf("history event count = %d", len(got))
	}
	if got[0].Type != "user_message" || got[0].Text != "hello" {
		t.Fatalf("user event = %+v", got[0])
	}
	if got[1].Type != "delta" || got[1].Kind != "thinking" {
		t.Fatalf("thinking event = %+v", got[1])
	}
	if got[2].Type != "tool_start" || got[2].ID != "call-1" {
		t.Fatalf("tool start = %+v", got[2])
	}
	if got[3].Type != "tool_end" || got[3].ID != "call-1" {
		t.Fatalf("tool end = %+v", got[3])
	}
	if got[4].Type != "message_end" || got[4].Text != "finished" {
		t.Fatalf("assistant event = %+v", got[4])
	}
}
