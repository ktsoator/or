package engine

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestToolRuntimeOwnsCatalogAndSkillAvailability(t *testing.T) {
	external := tools.Tool{AgentTool: agent.AgentTool{
		Definition: llm.ToolDefinition{Name: "external_tool"},
	}}
	session, err := New(t.Context(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   t.TempDir(),
		Tools: []tools.Tool{external},
		Skills: []skills.Skill{{
			Name: "review", Description: "review changes", Content: "review",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	assertAgentToolNames(t, session.agent.Snapshot(), "external_tool", skills.ToolName)
	if _, ok := session.toolRuntime.lookup("external_tool"); !ok {
		t.Fatal("runtime catalog omitted the configured tool")
	}
	if _, ok := session.toolRuntime.lookup(skills.ToolName); !ok {
		t.Fatal("runtime catalog omitted the hidden-capable Skill tool")
	}

	session.toolRuntime.setSkillAvailable(false)
	withoutSkill := session.agent.Snapshot()
	assertAgentToolNames(t, withoutSkill, "external_tool")
	if strings.Contains(withoutSkill.SystemPrompt, "## Skills") {
		t.Fatal("stable prompt kept the Skill protocol after the runtime hid its tool")
	}
	if _, ok := session.toolRuntime.lookup(skills.ToolName); !ok {
		t.Fatal("hiding the Skill tool removed it from the authorization catalog")
	}

	session.toolRuntime.setSkillAvailable(true)
	withSkill := session.agent.Snapshot()
	assertAgentToolNames(t, withSkill, "external_tool", skills.ToolName)
	if !strings.Contains(withSkill.SystemPrompt, "## Skills") {
		t.Fatal("stable prompt did not restore the Skill protocol with its tool")
	}
}

func assertAgentToolNames(t *testing.T, state agent.State, want ...string) {
	t.Helper()
	if len(state.Tools) != len(want) {
		t.Fatalf("agent tools = %#v, want %v", state.Tools, want)
	}
	for index, name := range want {
		if state.Tools[index].Definition.Name != name {
			t.Fatalf("agent tool %d = %q, want %q", index, state.Tools[index].Definition.Name, name)
		}
	}
}
