package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemContainsStableProtocols(t *testing.T) {
	out := BuildSystem(SystemOptions{
		WorkspaceRoot: "/repo",
		Tools: []ToolInfo{
			{Name: "read", Guidelines: []string{"Inspect before editing."}},
			{Name: "skill"},
		},
	})

	for _, want := range []string{
		DefaultInstructions,
		`- Root: "/repo"`,
		"## Tool guidelines",
		"- Inspect before editing.",
		"## Working rules",
		"## Approvals",
		"## Project context protocol",
		"`<or-context>`",
		"`context_update`",
		"## Skills",
		"call the `skill` tool",
		"## Response style",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
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

func TestBuildSystemDoesNotContainDynamicResourceSections(t *testing.T) {
	out := BuildSystem(SystemOptions{})
	for _, unwanted := range []string{
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
		Instructions:  "\nCustom instructions.\n",
		WorkspaceRoot: "/repo",
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
