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
	out := RenderBaseContext(Environment{}, []ContextFile{
		{Path: "/home/dev/.or/AGENTS.md", Content: "user", Scope: ScopeUser},
		{Path: "/repo/AGENTS.md", Content: "outer", Scope: ScopeProject},
		{Path: "/repo/AGENTS.local.md", Content: "local", Scope: ScopeLocal},
	})
	if !strings.Contains(out, `kind="base"`) {
		t.Fatalf("missing base metadata:\n%s", out)
	}
	for _, want := range []string{
		`scope="user" path="/home/dev/.or/AGENTS.md"`,
		`scope="project" path="/repo/AGENTS.md"`,
		`scope="local" path="/repo/AGENTS.local.md"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing instruction file %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "/home/dev/.or/AGENTS.md") > strings.Index(out, "/repo/AGENTS.md") ||
		strings.Index(out, "/repo/AGENTS.md") > strings.Index(out, "/repo/AGENTS.local.md") {
		t.Errorf("instruction precedence order changed:\n%s", out)
	}
}

func TestRenderBaseContextIncludesEnvironment(t *testing.T) {
	out := RenderBaseContext(Environment{
		OS:        "darwin",
		Arch:      "arm64",
		Shell:     "/bin/zsh",
		Date:      "2026-07-24",
		GitRepo:   true,
		GitBranch: "main",
	}, nil)
	for _, want := range []string{
		"<os>darwin</os>",
		"<arch>arm64</arch>",
		"<shell>/bin/zsh</shell>",
		"<date>2026-07-24</date>",
		"<git-repo>true</git-repo>",
		"<git-branch>main</git-branch>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("environment missing %q:\n%s", want, out)
		}
	}
}

func TestRenderBaseContextReportsAbsentRepositoryWithoutABranch(t *testing.T) {
	out := RenderBaseContext(Environment{OS: "linux", Date: "2026-07-24"}, nil)
	if !strings.Contains(out, "<git-repo>false</git-repo>") {
		t.Errorf("missing git-repo field:\n%s", out)
	}
	if strings.Contains(out, "<git-branch>") {
		t.Errorf("a non-repository must not claim a branch:\n%s", out)
	}
}

func TestRenderBaseContextTruncatesAnOversizedInstructionFile(t *testing.T) {
	long := strings.Repeat("x", maxContextFileChars+500)
	out := RenderBaseContext(Environment{}, []ContextFile{
		{Path: "/repo/AGENTS.md", Content: long, Scope: ScopeProject},
	})
	if strings.Contains(out, long) {
		t.Errorf("an oversized instruction file was projected in full:\n%s", out)
	}
	if !strings.Contains(out, `truncated="true"`) ||
		!strings.Contains(out, "[truncated: 500 more character(s) not shown") {
		t.Errorf("truncation was not announced:\n%s", out)
	}
}

func TestRenderInitialContextEmptyWhenNoUsableResources(t *testing.T) {
	base := RenderBaseContext(
		Environment{},
		[]ContextFile{{Path: "/repo/AGENTS.md", Content: " \n"}},
	)
	listing := RenderSkillListing("revision", []SkillInfo{{Name: "  ", Description: "ignored"}})
	if base != "" || listing != "" {
		t.Errorf("empty contexts = %q and %q, want empty", base, listing)
	}
}

func TestRenderContextUpdateCarriesCompleteCurrentState(t *testing.T) {
	env := Environment{OS: "darwin", Date: "2026-07-25", GitRepo: true, GitBranch: "feature"}
	files := []ContextFile{{Path: "/repo/AGENTS.md", Content: "new rule", Scope: ScopeProject}}
	out := RenderContextUpdate(ContextRevision(env, files), env, files)
	for _, want := range []string{
		`kind="context_update"`,
		"replaces the earlier base context",
		"<date>2026-07-25</date>",
		"<git-branch>feature</git-branch>",
		`scope="project" path="/repo/AGENTS.md"`,
		"new rule",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("context update missing %q:\n%s", want, out)
		}
	}
}

func TestContextRevisionTracksEveryModelVisibleInput(t *testing.T) {
	env := Environment{OS: "darwin", Date: "2026-07-24", GitRepo: true, GitBranch: "main"}
	files := []ContextFile{{Path: "/repo/AGENTS.md", Content: "rule", Scope: ScopeProject}}
	base := ContextRevision(env, files)

	branched := env
	branched.GitBranch = "other"
	edited := []ContextFile{{Path: "/repo/AGENTS.md", Content: "rule two", Scope: ScopeProject}}
	for name, revision := range map[string]string{
		"branch change": ContextRevision(branched, files),
		"file edit":     ContextRevision(env, edited),
	} {
		if revision == base {
			t.Errorf("%s did not advance the revision", name)
		}
	}
	if ContextRevision(env, files) != base {
		t.Error("revision is not stable for identical inputs")
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
	base := RenderBaseContext(Environment{}, []ContextFile{{
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
	env := Environment{OS: "darwin", Arch: "arm64", Date: "2026-07-24"}
	firstBase := RenderBaseContext(env, files)
	firstListing := RenderSkillListing("revision", skills)
	for range 10 {
		if got := RenderBaseContext(env, files); got != firstBase {
			t.Fatalf("base render changed:\nfirst:\n%s\nnext:\n%s", firstBase, got)
		}
		if got := RenderSkillListing("revision", skills); got != firstListing {
			t.Fatalf("listing render changed:\nfirst:\n%s\nnext:\n%s", firstListing, got)
		}
	}
}
