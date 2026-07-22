package prompt

import (
	"strings"
	"testing"
)

func TestBuildIncludesSkillsSection(t *testing.T) {
	out := Build(Options{
		Skills: []SkillInfo{
			{Name: "commit", Description: "Use when committing changes"},
			{Name: "review", Description: "Use when reviewing a diff"},
		},
	})
	if !strings.Contains(out, "## Available skills") {
		t.Fatalf("missing skills section:\n%s", out)
	}
	if !strings.Contains(out, "- commit: Use when committing changes") {
		t.Errorf("missing commit entry:\n%s", out)
	}
	if !strings.Contains(out, "- review: Use when reviewing a diff") {
		t.Errorf("missing review entry:\n%s", out)
	}
	if !strings.Contains(out, "call the skill tool") {
		t.Errorf("section should instruct calling the skill tool:\n%s", out)
	}
}

func TestBuildOmitsSkillsSectionWhenEmpty(t *testing.T) {
	out := Build(Options{})
	if strings.Contains(out, "Available skills") {
		t.Errorf("skills section should be omitted when no skills:\n%s", out)
	}
}

func TestSkillDescriptionTruncated(t *testing.T) {
	long := strings.Repeat("x", maxSkillDescChars+50)
	out := Build(Options{Skills: []SkillInfo{{Name: "big", Description: long}}})
	// The entry line is truncated to the cap (with an ellipsis) rather than
	// emitting the full description.
	if strings.Contains(out, long) {
		t.Errorf("long description should be truncated:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("truncation should append an ellipsis:\n%s", out)
	}
}

func TestSkillWithoutNameSkipped(t *testing.T) {
	out := Build(Options{Skills: []SkillInfo{{Name: "  ", Description: "d"}}})
	if strings.Contains(out, "Available skills") {
		t.Errorf("a nameless skill should not produce a section:\n%s", out)
	}
}
