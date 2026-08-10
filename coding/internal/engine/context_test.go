package engine

import (
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/llm"
)

func TestEstimateContextBreakdownCalibratesCategoriesToMeasuredTotal(t *testing.T) {
	state := agent.State{
		SystemPrompt: "stable system instructions for the coding agent",
		Tools: []agent.AgentTool{{Definition: llm.ToolDefinition{
			Name:        "read",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		}}},
		Messages: []agent.AgentMessage{
			agent.FromLLM(llm.UserText("inspect the workspace")),
			agent.FromLLM(llm.AssistantText("I will inspect it.")),
		},
	}
	attachments := []modelcontext.Attachment{
		{Kind: modelcontext.BaseContext, Rendered: "workspace environment and AGENTS.md"},
		{Kind: modelcontext.SkillListing, Rendered: "available Skills"},
		{Kind: modelcontext.ActivatedSkill, Rendered: "activated Skill instructions"},
		{Kind: modelcontext.TaskStatus, Rendered: "background task status"},
	}

	breakdown := estimateContextBreakdown(state, attachments, 10_000)
	if breakdown == nil {
		t.Fatal("breakdown is nil")
	}
	if breakdown.total() != 10_000 {
		t.Fatalf("breakdown total = %d, want 10000", breakdown.total())
	}
	if breakdown.Messages <= 0 || breakdown.SystemTools <= 0 ||
		breakdown.SystemPrompt <= 0 || breakdown.Skills <= 0 ||
		breakdown.ProjectContext <= 0 {
		t.Fatalf("breakdown = %#v, want every populated category to be positive", breakdown)
	}
}

func TestEstimateTextTokensWeightsNonASCIIRunes(t *testing.T) {
	if got := estimateTextTokens("abcd"); got != 1 {
		t.Fatalf("ASCII estimate = %d, want 1", got)
	}
	if got := estimateTextTokens("上下文"); got != 3 {
		t.Fatalf("non-ASCII estimate = %d, want 3", got)
	}
}
