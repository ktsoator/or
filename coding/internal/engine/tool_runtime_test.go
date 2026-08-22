package engine

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
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

func TestToolRuntimeFiltersToolsByModelAndRefreshesOnSetModel(t *testing.T) {
	textTool := tools.Tool{AgentTool: agent.AgentTool{
		Definition: llm.ToolDefinition{Name: "text_tool"},
	}}
	imageTool := tools.Tool{
		AgentTool:      agent.AgentTool{Definition: llm.ToolDefinition{Name: "image_tool"}},
		RequiredInputs: []llm.ModelInput{llm.ModelInputImage},
		Guidelines:     []string{"vision-only guideline"},
	}
	session, err := New(t.Context(), Options{
		Model: llm.Model{Provider: "test", ID: "text", Input: []llm.ModelInput{llm.ModelInputText}},
		Cwd:   t.TempDir(),
		Tools: []tools.Tool{textTool, imageTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	initial := session.Snapshot()
	assertAgentToolNames(t, initial, "text_tool")
	if strings.Contains(initial.SystemPrompt, "vision-only guideline") {
		t.Fatal("text-only prompt contains the image tool guideline")
	}

	vision := llm.Model{
		Provider: "test", ID: "vision",
		Input: []llm.ModelInput{llm.ModelInputText, llm.ModelInputImage},
	}
	session.SetModel(vision)
	withVision := session.Snapshot()
	assertAgentToolNames(t, withVision, "text_tool", "image_tool")
	if withVision.Model.ID != vision.ID {
		t.Fatalf("agent model = %q, want %q", withVision.Model.ID, vision.ID)
	}
	if !strings.Contains(withVision.SystemPrompt, "vision-only guideline") {
		t.Fatal("vision prompt omitted the image tool guideline")
	}

	session.SetModel(llm.Model{Provider: "test", ID: "unknown"})
	unknown := session.Snapshot()
	assertAgentToolNames(t, unknown, "text_tool")
	if strings.Contains(unknown.SystemPrompt, "vision-only guideline") {
		t.Fatal("unknown-modality prompt contains the image tool guideline")
	}
}

func TestToolRuntimeBlocksIncompatibleToolBeforeAuthorization(t *testing.T) {
	accessChecked := false
	imageTool := tools.Tool{
		AgentTool:      agent.AgentTool{Definition: llm.ToolDefinition{Name: "image_tool"}},
		RequiredInputs: []llm.ModelInput{llm.ModelInputImage},
		AccessFor: func(map[string]any) []permission.Access {
			accessChecked = true
			return []permission.Access{{Action: permission.Read, Path: "missing.png"}}
		},
	}
	session, err := New(t.Context(), Options{
		Model: llm.Model{Provider: "test", ID: "text-only", Input: []llm.ModelInput{llm.ModelInputText}},
		Cwd:   t.TempDir(),
		Tools: []tools.Tool{imageTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	blocked, reason := session.toolRuntime.beforeToolCall(agent.BeforeToolCallCtx{
		RunContext: t.Context(),
		ToolCall:   llm.ToolCall{ID: "call-1", Name: "image_tool"},
		Args:       map[string]any{"path": "missing.png"},
	})
	if !blocked || !strings.Contains(reason, "image") {
		t.Fatalf("beforeToolCall() = blocked %v, reason %q", blocked, reason)
	}
	if accessChecked {
		t.Fatal("incompatible tool reached authorization")
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
