package engine

import (
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestProjectHistoryPreservesInterruptedAssistantContent(t *testing.T) {
	tests := []struct {
		name        string
		content     []llm.AssistantContent
		wantText    string
		wantHistory int
	}{
		{
			name: "thinking without response text",
			content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "Inspect the existing implementation."},
				&llm.ToolCall{ID: "call-1", Name: "write", Arguments: map[string]any{"path": "note.txt"}},
			},
			wantHistory: 2,
		},
		{
			name: "thinking with partial response text",
			content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "Inspect the existing implementation."},
				&llm.TextContent{Text: "I found the relevant code."},
				&llm.ToolCall{ID: "call-1", Name: "write", Arguments: map[string]any{"path": "note.txt"}},
			},
			wantText:    "I found the relevant code.",
			wantHistory: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := projectHistory([]agent.AgentMessage{agent.FromLLM(&llm.AssistantMessage{
				Content: tt.content, StopReason: llm.StopReasonAborted,
				Provider: "openai", Model: "gpt-5",
			})}, nil)

			if len(items) != tt.wantHistory {
				t.Fatalf("history = %#v, want %d items", items, tt.wantHistory)
			}
			if items[0].Type != HistoryThinking || items[0].Text != "Inspect the existing implementation." {
				t.Fatalf("thinking = %#v", items[0])
			}
			last := items[len(items)-1]
			if last.Type != HistoryAssistant || last.Text != tt.wantText || last.FinalResponse {
				t.Fatalf("assistant = %#v", last)
			}
			for _, item := range items {
				if item.Type == HistoryToolCall {
					t.Fatalf("interrupted tool call was restored: %#v", item)
				}
			}
		})
	}
}
