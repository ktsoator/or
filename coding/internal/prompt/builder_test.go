package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemContainsStableProtocols(t *testing.T) {
	out := BuildSystem(SystemOptions{
		Tools: []ToolInfo{
			{Name: "read", Guidelines: []string{"Inspect before editing."}},
			{Name: "skill"},
		},
	})

	for _, want := range []string{
		DefaultInstructions,
		"## Tool guidelines",
		"- Inspect before editing.",
		"## Working rules",
		"## Approvals",
		"## Session context protocol",
		"`<or-context>`",
		"`context_update`",
		"## Skills",
		"call the `skill` tool",
		"## User updates",
		"## Response style",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestBuildSystemKeepsUserUpdatesWithCustomInstructions(t *testing.T) {
	out := BuildSystem(SystemOptions{Instructions: "Custom instructions."})

	for _, want := range []string{
		"## User updates",
		"before the first tool call",
		"meaningful discoveries or completed milestones",
		"more than 60 seconds",
		"Do not narrate each tool call",
		"keep the final response self-contained",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing user-update rule %q:\n%s", want, out)
		}
	}
}

// A tool's own description travels in its schema, which every request already
// carries. Repeating it in the system prompt would spend tokens on a second copy
// that can drift from the first.
func TestBuildSystemDoesNotRestateToolDescriptions(t *testing.T) {
	out := BuildSystem(SystemOptions{
		Tools: []ToolInfo{{Name: "read"}, {Name: "grep"}},
	})
	if strings.Contains(out, "## Available tools") {
		t.Errorf("system prompt duplicates the tool schemas:\n%s", out)
	}
}

func TestBuildSystemOmitsSkillProtocolWithoutTool(t *testing.T) {
	out := BuildSystem(SystemOptions{Tools: []ToolInfo{{Name: "read"}}})
	if strings.Contains(out, "## Skills") {
		t.Errorf("skill protocol should follow the active tool set:\n%s", out)
	}
}

func TestBuildSystemIncludesPlanModePolicyOnlyWhenActive(t *testing.T) {
	inactive := BuildSystem(SystemOptions{})
	if strings.Contains(inactive, "## Plan mode") {
		t.Fatalf("inactive prompt contains plan-mode policy:\n%s", inactive)
	}

	active := BuildSystem(SystemOptions{PlanMode: true})
	for _, want := range []string{
		"## Plan mode",
		"do not edit files",
		"Do not call todo_write",
		"only and final tool call",
		"exit_plan_mode",
	} {
		if !strings.Contains(active, want) {
			t.Errorf("active prompt missing plan-mode rule %q:\n%s", want, active)
		}
	}
}

func TestBuildSystemDoesNotContainDynamicResourceSections(t *testing.T) {
	out := BuildSystem(SystemOptions{})
	for _, unwanted := range []string{
		"user's workspace",
		"## Workspace",
		"## Working directory",
		"<instruction-file",
		"<available-skills>",
		"## Project context:",
		"## Available skills",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("stable system prompt contains dynamic marker %q:\n%s", unwanted, out)
		}
	}
}

func TestBuildSystemIsDeterministic(t *testing.T) {
	opts := SystemOptions{
		Instructions: "\nCustom instructions.\n",
		Tools: []ToolInfo{
			{Name: "read", Guidelines: []string{"First.", "Shared."}},
			{Name: "edit", Guidelines: []string{"Shared.", "Second."}},
		},
	}
	first := BuildSystem(opts)
	for range 10 {
		if got := BuildSystem(opts); got != first {
			t.Fatalf("build changed:\nfirst:\n%s\nnext:\n%s", first, got)
		}
	}
	if strings.Count(first, "- Shared.") != 1 {
		t.Fatalf("duplicate guideline was not removed:\n%s", first)
	}
}
