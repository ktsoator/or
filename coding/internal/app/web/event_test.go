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
