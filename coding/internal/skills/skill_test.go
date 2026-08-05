package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// writeSkill creates <root>/<dir>/SKILL.md with the given contents.
func writeSkill(t *testing.T, root, dir, contents string) string {
	t.Helper()
	skillDir := filepath.Join(root, dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(skillDir, skillFile)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillDir
}

const commitSkill = `---
name: commit
description: Use when the user asks to commit the current changes
---

# Commit changes

Check status and diff first.

$ARGUMENTS
`

func TestLoadParsesFrontmatterAndBody(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", commitSkill)

	reg, diags := Load(LoadOptions{UserDir: root})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	s, ok := reg.Lookup("commit")
	if !ok {
		t.Fatal("commit not found")
	}
	if s.Description != "Use when the user asks to commit the current changes" {
		t.Errorf("description = %q", s.Description)
	}
	if s.Source != SourceUser {
		t.Errorf("source = %q, want user", s.Source)
	}
	if !strings.HasPrefix(s.Content, "# Commit changes") {
		t.Errorf("body should start with heading, got %q", s.Content)
	}
	if strings.Contains(s.Content, "---") {
		t.Errorf("body should not contain frontmatter fence: %q", s.Content)
	}
}

func TestLoadParsesDisableModelInvocation(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy", `---
name: deploy
description: Deploy the application
disable-model-invocation: true
---

Deploy $ARGUMENTS.
`)

	reg, diags := Load(LoadOptions{UserDir: root})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	s, ok := reg.Lookup("deploy")
	if !ok {
		t.Fatal("manual skill was removed from the complete registry")
	}
	if !s.DisableModelInvocation {
		t.Fatal("disable-model-invocation was not parsed")
	}
	if _, ok := reg.ModelLookup("deploy"); ok || len(reg.ModelList()) != 0 {
		t.Fatal("manual skill remains visible to the model")
	}
}

func TestProjectOverridesUser(t *testing.T) {
	userRoot := t.TempDir()
	projectRoot := t.TempDir()
	writeSkill(t, userRoot, "commit", commitSkill)
	writeSkill(t, projectRoot, "commit", `---
name: commit
description: project-specific commit skill
---

project body
`)

	reg, diags := Load(LoadOptions{UserDir: userRoot, ProjectDir: projectRoot})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	s, _ := reg.Lookup("commit")
	if s.Source != SourceProject {
		t.Errorf("source = %q, want project override", s.Source)
	}
	if s.Description != "project-specific commit skill" {
		t.Errorf("description = %q, want project version", s.Description)
	}
	if reg.Len() != 1 {
		t.Errorf("Len = %d, want 1 (override, not duplicate)", reg.Len())
	}
}

func TestListIsSortedAndStable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"review", "commit", "explain"} {
		writeSkill(t, root, name, "---\nname: "+name+"\ndescription: d\n---\nbody\n")
	}
	reg, _ := Load(LoadOptions{UserDir: root})
	var got []string
	for _, s := range reg.List() {
		got = append(got, s.Name)
	}
	want := []string{"commit", "explain", "review"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("List order = %v, want %v", got, want)
	}
}

func TestNameMustMatchDirectory(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", "---\nname: not-commit\ndescription: d\n---\nbody\n")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 {
		t.Errorf("mismatched skill should be skipped, got %d", reg.Len())
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "must match directory name") {
		t.Errorf("expected name-mismatch diagnostic, got %+v", diags)
	}
}

func TestMissingDescriptionIsDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", "---\nname: commit\n---\nbody\n")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 {
		t.Errorf("skill without description should be skipped")
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "description") {
		t.Errorf("expected missing-description diagnostic, got %+v", diags)
	}
}

func TestMissingFrontmatterIsDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", "# no frontmatter here\n")
	_, diags := Load(LoadOptions{UserDir: root})
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "frontmatter") {
		t.Errorf("expected frontmatter diagnostic, got %+v", diags)
	}
}

func TestDirectoryWithoutSkillFileIgnored(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notaskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 0 {
		t.Errorf("directory without SKILL.md should be silently ignored, got len=%d diags=%+v", reg.Len(), diags)
	}
}

func TestAbsentRootIsNotAnError(t *testing.T) {
	reg, diags := Load(LoadOptions{UserDir: filepath.Join(t.TempDir(), "does-not-exist")})
	if reg.Len() != 0 || len(diags) != 0 {
		t.Errorf("absent root should be a no-op, got len=%d diags=%+v", reg.Len(), diags)
	}
}

func TestExpandPlaceholders(t *testing.T) {
	body := "dir=${OR_SKILL_DIR} all=$ARGUMENTS first=$1 second=$2 at=$@"
	got := Expand(body, "/skills/commit", "alpha beta")
	want := "dir=/skills/commit all=alpha beta first=alpha second=beta at=alpha beta"
	if got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestExpandMissingPositionalIsEmpty(t *testing.T) {
	got := Expand("[$1][$2]", "/d", "only")
	if got != "[only][]" {
		t.Errorf("Expand = %q, want [only][]", got)
	}
}

func TestToolLoadsSkillBody(t *testing.T) {
	root := t.TempDir()
	dir := writeSkill(t, root, "commit", commitSkill)
	reg, _ := Load(LoadOptions{UserDir: root})
	tool := reg.Tool()

	args, _ := json.Marshal(skillCallArgs{Name: "commit", Arguments: "stage everything"})
	res, err := tool.Execute(t.Context(), "call-1", args, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "<loaded_skill name=\"commit\"") {
		t.Errorf("missing loaded_skill wrapper: %q", text)
	}
	if !strings.Contains(text, "root=\""+dir+"\"") {
		t.Errorf("wrapper should carry skill dir %q: %q", dir, text)
	}
	if !strings.Contains(text, relativePathProtocol) {
		t.Errorf("missing relative-path protocol: %q", text)
	}
	if !strings.Contains(text, "stage everything") {
		t.Errorf("$ARGUMENTS not expanded: %q", text)
	}
	if !strings.Contains(text, "# Commit changes") {
		t.Errorf("body not injected: %q", text)
	}
}

func TestToolUnknownSkillReturnsError(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", commitSkill)
	reg, _ := Load(LoadOptions{UserDir: root})
	tool := reg.Tool()

	args, _ := json.Marshal(skillCallArgs{Name: "nope"})
	res, err := tool.Execute(t.Context(), "call-1", args, nil)
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Unknown skill") || !strings.Contains(text, "commit") {
		t.Errorf("error should name valid skills, got %q", text)
	}
}

func TestToolRejectsManualSkillWithoutRevealingItsName(t *testing.T) {
	reg := NewRegistry([]Skill{
		{Name: "review", Description: "review changes", Content: "review"},
		{
			Name:                   "deploy",
			Description:            "deploy the app",
			Content:                "deploy",
			DisableModelInvocation: true,
		},
	})
	tool := reg.Tool()

	args, _ := json.Marshal(skillCallArgs{Name: "deploy"})
	res, err := tool.Execute(t.Context(), "call-1", args, nil)
	if err == nil {
		t.Fatal("manual skill should not be callable by the model-facing tool")
	}
	text := resultText(t, res)
	if strings.Contains(text, "deploy") && strings.Contains(text, "Available skills: deploy") {
		t.Fatalf("manual skill leaked into valid names: %q", text)
	}
	if !strings.Contains(text, "Available skills: review") {
		t.Fatalf("model-visible valid skill missing: %q", text)
	}
}

func TestExplicitInvocationExpandsManualSkill(t *testing.T) {
	reg := NewRegistry([]Skill{{
		Name:                   "deploy",
		Description:            "deploy the app",
		Content:                "target=$ARGUMENTS root=${OR_SKILL_DIR}",
		Dir:                    "/skills/deploy",
		DisableModelInvocation: true,
	}})

	expanded, matched, err := reg.ExpandExplicitInvocation("/skill:deploy staging")
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("explicit invocation was not recognized")
	}
	for _, want := range []string{
		explicitInvocationPrefix,
		`name="deploy"`,
		"already loaded; do not call the skill tool again",
		"target=staging root=/skills/deploy",
		relativePathProtocol,
	} {
		if !strings.Contains(expanded, want) {
			t.Errorf("expanded invocation missing %q:\n%s", want, expanded)
		}
	}
	if !IsExplicitInvocationText(expanded) {
		t.Fatal("expanded block was not recognizable by UI projection")
	}
}

func TestDisplayExplicitInvocation(t *testing.T) {
	registry := NewRegistry([]Skill{{
		Name: "deploy", Dir: "/skills/deploy", Path: "/skills/deploy/SKILL.md",
	}, {
		Name: "review", Dir: "/skills/review path", Path: "/skills/review path/SKILL.md",
	}})
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "/skill:deploy staging", want: "[$deploy](/skills/deploy/SKILL.md) staging"},
		{input: "  /skill:review  ", want: "[$review](</skills/review path/SKILL.md>)"},
		{input: "[$deploy](/old/SKILL.md) staging", want: "[$deploy](/skills/deploy/SKILL.md) staging"},
		{input: "/review staging", want: "/review staging"},
	} {
		if got := registry.DisplayExplicitInvocation(tt.input); got != tt.want {
			t.Errorf("DisplayExplicitInvocation(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// resultText extracts the concatenated text of a tool result.
func resultText(t *testing.T, res agent.ToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*llm.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
