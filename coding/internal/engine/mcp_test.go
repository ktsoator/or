package engine

import (
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestNewAppendsProductIntegrationTools(t *testing.T) {
	additional := tools.Tool{AgentTool: agent.AgentTool{Definition: llm.ToolDefinition{Name: "external_tool"}}}
	session, err := New(t.Context(), Options{
		Model:           llm.Model{Provider: "test", ID: "model"},
		Cwd:             t.TempDir(),
		Tools:           []tools.Tool{},
		AdditionalTools: []tools.Tool{additional},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, ok := session.toolByName["external_tool"]; !ok {
		t.Fatal("additional tool was not added")
	}
}
