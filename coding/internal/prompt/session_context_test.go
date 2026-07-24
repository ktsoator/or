package prompt

import (
	"strings"
	"testing"
)

func TestRenderSessionContextIncludesSkills(t *testing.T) {
	out := RenderSessionContext(SessionContextOptions{
		Skills: []SkillInfo{
			{Name: "review", Description: "Use when reviewing a diff"},
			{Name: "commit", Description: "Use when committing changes"},
		},
	})
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

func TestRenderSessionContextIncludesInstructionFilesInInputOrder(t *testing.T) {
	out := RenderSessionContext(SessionContextOptions{
		ContextFiles: []ContextFile{
			{Path: "/repo/AGENTS.md", Content: "outer", Scope: ScopeProject},
			{Path: "/repo/service/AGENTS.md", Content: "inner", Scope: ScopeNested},
		},
	})
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

func TestRenderSessionContextEmptyWhenNoUsableResources(t *testing.T) {
	out := RenderSessionContext(SessionContextOptions{
		ContextFiles: []ContextFile{{Path: "/repo/AGENTS.md", Content: " \n"}},
		Skills:       []SkillInfo{{Name: "  ", Description: "ignored"}},
	})
	if out != "" {
		t.Errorf("empty context = %q, want empty", out)
	}
}

func TestRenderSessionContextTruncatesSkillDescription(t *testing.T) {
	long := strings.Repeat("x", maxSkillDescChars+50)
	out := RenderSessionContext(SessionContextOptions{
		Skills: []SkillInfo{{Name: "big", Description: long}},
	})
	// The entry line is truncated to the cap (with an ellipsis) rather than
	// emitting the full description.
	if strings.Contains(out, long) {
		t.Errorf("long description should be truncated:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("truncation should append an ellipsis:\n%s", out)
	}
}

func TestRenderSessionContextEscapesMetadata(t *testing.T) {
	out := RenderSessionContext(SessionContextOptions{
		ContextFiles: []ContextFile{{
			Path:    `/repo/"quoted"&file`,
			Content: "instructions",
		}},
		Skills: []SkillInfo{{
			Name:        "review&check",
			Description: "Use <carefully>",
		}},
	})
	for _, want := range []string{
		`path="/repo/&#34;quoted&#34;&amp;file"`,
		"<name>review&amp;check</name>",
		"<description>Use &lt;carefully&gt;</description>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing escaped value %q:\n%s", want, out)
		}
	}
}

func TestRenderSessionContextIsDeterministic(t *testing.T) {
	opts := SessionContextOptions{
		ContextFiles: []ContextFile{{
			Path: "/repo/AGENTS.md", Content: "instructions\n", Scope: ScopeProject,
		}},
		Skills: []SkillInfo{
			{Name: "zeta", Description: "last"},
			{Name: "alpha", Description: "first"},
		},
	}
	first := RenderSessionContext(opts)
	for range 10 {
		if got := RenderSessionContext(opts); got != first {
			t.Fatalf("render changed:\nfirst:\n%s\nnext:\n%s", first, got)
		}
	}
}
