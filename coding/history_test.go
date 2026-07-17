package coding

import (
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/tools"
	"github.com/ktsoator/or/llm"
)

func TestProjectHistoryAttachesDetails(t *testing.T) {
	messages := []agent.AgentMessage{
		agent.FromLLM(&llm.ToolResultMessage{
			ToolCallID: "c1",
			ToolName:   "edit",
			Content:    []llm.ToolResultContent{&llm.TextContent{Text: "Edited a.go"}},
		}),
		agent.FromLLM(&llm.ToolResultMessage{
			ToolCallID: "c2",
			ToolName:   "bash",
			Content:    []llm.ToolResultContent{&llm.TextContent{Text: "ok"}},
		}),
	}
	details := map[string]any{
		"c1": tools.FileChange{Path: "a.go", Kind: tools.ChangeUpdate, Additions: 1},
	}

	items := projectHistory(messages, details)

	var edit, bash *HistoryItem
	for i := range items {
		switch items[i].ToolCallID {
		case "c1":
			edit = &items[i]
		case "c2":
			bash = &items[i]
		}
	}
	if edit == nil || bash == nil {
		t.Fatalf("missing tool-result items: %+v", items)
	}
	if _, ok := edit.ToolDetails.(tools.FileChange); !ok {
		t.Fatalf("edit ToolDetails = %T, want tools.FileChange", edit.ToolDetails)
	}
	if bash.ToolDetails != nil {
		t.Fatalf("bash ToolDetails = %v, want nil", bash.ToolDetails)
	}
}
