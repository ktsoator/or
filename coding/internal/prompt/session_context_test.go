package prompt

import (
	"strings"
	"testing"
)

func TestRenderSkillListingIncludesSkills(t *testing.T) {
	out := RenderSkillListing("revision-1", []SkillInfo{
		{Name: "review", Description: "Use when reviewing a diff"},
		{Name: "commit", Description: "Use when committing changes"},
	})
	if !strings.Contains(out, `kind="skill_listing" revision="revision-1"`) {
		t.Fatalf("missing listing metadata:\n%s", out)
	}
	if !strings.Contains(out, "<available-skills>") {
		t.Fatalf("missing skills section:\n%s", out)
	}
	if !strings.Contains(out, "<name>commit</name>") ||
		!strings.Contains(out, "<description>Use when committing changes</description>") {
		t.Errorf("missing commit entry:\n%s", out)
	}
	if !strings.Contains(out, "<name>review</name>") ||
		!strings.Contains(out, "<description>Use when reviewing a diff</description>") {
		t.Errorf("missing review entry:\n%s", out)
	}
	if strings.Index(out, "<name>commit</name>") > strings.Index(out, "<name>review</name>") {
		t.Errorf("skills should be rendered in stable name order:\n%s", out)
	}
}

func TestRenderBaseContextIncludesInstructionFilesInInputOrder(t *testing.T) {
	out := RenderBaseContext([]ContextFile{
		{Path: "/repo/AGENTS.md", Content: "outer", Scope: ScopeProject},
		{Path: "/repo/service/AGENTS.md", Content: "inner", Scope: ScopeNested},
	})
	if !strings.Contains(out, `kind="base"`) {
		t.Fatalf("missing base metadata:\n%s", out)
	}
	if !strings.Contains(out, `scope="project" path="/repo/AGENTS.md"`) {
		t.Fatalf("missing project instruction file:\n%s", out)
	}
	if !strings.Contains(out, `scope="nested" path="/repo/service/AGENTS.md"`) {
		t.Fatalf("missing nested instruction file:\n%s", out)
	}
	if strings.Index(out, "/repo/AGENTS.md") > strings.Index(out, "/repo/service/AGENTS.md") {
		t.Errorf("instruction precedence order changed:\n%s", out)
	}
}

func TestRenderInitialContextEmptyWhenNoUsableResources(t *testing.T) {
	base := RenderBaseContext([]ContextFile{{Path: "/repo/AGENTS.md", Content: " \n"}})
	listing := RenderSkillListing("revision", []SkillInfo{{Name: "  ", Description: "ignored"}})
	if base != "" || listing != "" {
		t.Errorf("empty contexts = %q and %q, want empty", base, listing)
	}
}

func TestRenderSkillListingTruncatesDescription(t *testing.T) {
	long := strings.Repeat("x", maxSkillDescChars+50)
	out := RenderSkillListing("revision", []SkillInfo{{Name: "big", Description: long}})
	// The entry line is truncated to the cap (with an ellipsis) rather than
	// emitting the full description.
	if strings.Contains(out, long) {
		t.Errorf("long description should be truncated:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("truncation should append an ellipsis:\n%s", out)
	}
}

func TestRenderContextEscapesMetadata(t *testing.T) {
	base := RenderBaseContext([]ContextFile{{
		Path:    `/repo/"quoted"&file`,
		Content: "instructions",
	}})
	listing := RenderSkillListing("revision&one", []SkillInfo{{
		Name:        "review&check",
		Description: "Use <carefully>",
	}})
	for _, test := range []struct {
		output string
		want   string
	}{
		{base, `path="/repo/&#34;quoted&#34;&amp;file"`},
		{listing, `revision="revision&amp;one"`},
		{listing, "<name>review&amp;check</name>"},
		{listing, "<description>Use &lt;carefully&gt;</description>"},
	} {
		if !strings.Contains(test.output, test.want) {
			t.Errorf("missing escaped value %q:\n%s", test.want, test.output)
		}
	}
}

func TestRenderSkillsUpdateIncludesDeltaAndCompleteCurrentState(t *testing.T) {
	out := RenderSkillsUpdate(
		"revision-2",
		[]SkillInfo{
			{Name: "added", Description: "new skill"},
			{Name: "updated", Description: "new description"},
		},
		SkillsDelta{
			Added:   []SkillInfo{{Name: "added", Description: "new skill"}},
			Updated: []SkillInfo{{Name: "updated", Description: "new description"}},
			Removed: []string{"removed"},
		},
	)
	for _, want := range []string{
		`kind="skills_update" revision="revision-2"`,
		"<added>",
		"<updated>",
		"<removed>",
		"<name>removed</name>",
		"<available-skills>",
		"<name>added</name>",
		"<name>updated</name>",
		"replaces every earlier skill listing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("skills update missing %q:\n%s", want, out)
		}
	}
}

func TestRenderSkillsUpdateCanRemoveLastSkill(t *testing.T) {
	out := RenderSkillsUpdate(
		"empty",
		nil,
		SkillsDelta{Removed: []string{"last"}},
	)
	if !strings.Contains(out, `<available-skills none="true" />`) ||
		!strings.Contains(out, "<name>last</name>") {
		t.Fatalf("last-skill removal is not explicit:\n%s", out)
	}
}

func TestContextRenderingIsDeterministic(t *testing.T) {
	files := []ContextFile{{
		Path: "/repo/AGENTS.md", Content: "instructions\n", Scope: ScopeProject,
	}}
	skills := []SkillInfo{
		{Name: "zeta", Description: "last"},
		{Name: "alpha", Description: "first"},
	}
	firstBase := RenderBaseContext(files)
	firstListing := RenderSkillListing("revision", skills)
	for range 10 {
		if got := RenderBaseContext(files); got != firstBase {
			t.Fatalf("base render changed:\nfirst:\n%s\nnext:\n%s", firstBase, got)
		}
		if got := RenderSkillListing("revision", skills); got != firstListing {
			t.Fatalf("listing render changed:\nfirst:\n%s\nnext:\n%s", firstListing, got)
		}
	}
}
