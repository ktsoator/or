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

$ARGUMENTS and $1 remain literal skill instructions.
`

func TestLoadParsesStandardFrontmatterAndPreservesBody(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "pdf-processing", `---
name: pdf-processing
description: Extract PDF text, fill forms, and merge files. Use when handling PDFs.
license: Apache-2.0
compatibility: Requires Python 3.14+ and uv
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(uv:*) Read
---

# PDF processing

Run scripts/extract.py with "$1" and "$@".
`)

	reg, diags := Load(LoadOptions{UserDir: root})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	s, ok := reg.Lookup("pdf-processing")
	if !ok {
		t.Fatal("pdf-processing not found")
	}
	if s.License != "Apache-2.0" || s.Compatibility != "Requires Python 3.14+ and uv" {
		t.Errorf("optional strings = license %q compatibility %q", s.License, s.Compatibility)
	}
	if s.Metadata["author"] != "example-org" || s.Metadata["version"] != "1.0" {
		t.Errorf("metadata = %#v", s.Metadata)
	}
	if s.AllowedTools != "Bash(uv:*) Read" {
		t.Errorf("allowed tools = %q", s.AllowedTools)
	}
	wantBody := "\n# PDF processing\n\nRun scripts/extract.py with \"$1\" and \"$@\".\n"
	if s.Content != wantBody {
		t.Errorf("body changed:\ngot  %q\nwant %q", s.Content, wantBody)
	}
}

func TestLoadAcceptsSpecificationNames(t *testing.T) {
	for _, name := range []string{"pdf", "pdf-processing", "data-analysis", "code-review", strings.Repeat("a", 64)} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, name, "---\nname: "+name+"\ndescription: valid\n---\nbody")
			reg, diags := Load(LoadOptions{UserDir: root})
			if reg.Len() != 1 || len(diags) != 0 {
				t.Fatalf("load = %d skills, diagnostics %+v", reg.Len(), diags)
			}
		})
	}
}

func TestLoadRejectsInvalidSpecificationNames(t *testing.T) {
	for _, name := range []string{"PDF-Processing", "-pdf", "pdf-", "pdf--processing", "pdf_processing", strings.Repeat("a", 65)} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, name, "---\nname: "+name+"\ndescription: invalid\n---\nbody")
			reg, diags := Load(LoadOptions{UserDir: root})
			if reg.Len() != 0 || len(diags) != 1 {
				t.Fatalf("load = %d skills, diagnostics %+v", reg.Len(), diags)
			}
		})
	}
}

func TestLoadRejectsDescriptionOverLimit(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "review", "---\nname: review\ndescription: "+strings.Repeat("x", 1025)+"\n---\nbody")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "1024") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestLoadRejectsCompatibilityOverLimit(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "review", "---\nname: review\ndescription: review\ncompatibility: "+strings.Repeat("x", 501)+"\n---\nbody")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "500") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestLoadRejectsEmptyCompatibility(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "review", "---\nname: review\ndescription: review\ncompatibility: \"\"\n---\nbody")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "must not be empty") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestLoadRejectsUnknownFrontmatterField(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy", "---\nname: deploy\ndescription: deploy\ndisable-model-invocation: true\n---\nbody")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "unsupported field") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestLoadRejectsNonStringMetadataValue(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "review", "---\nname: review\ndescription: review\nmetadata:\n  version: 1\n---\nbody")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "must be a string") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestLoadRejectsNullMetadata(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "review", "---\nname: review\ndescription: review\nmetadata:\n---\nbody")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "must be a mapping") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestRootsUseOnlyAgentsSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)

	userDir, projectDir := Roots(workspace)
	if userDir != filepath.Join(home, ".agents", "skills") ||
		projectDir != filepath.Join(workspace, ".agents", "skills") {
		t.Fatalf("roots = %q, %q", userDir, projectDir)
	}
	writeSkill(t, filepath.Join(home, ".or", "skills"), "legacy", "---\nname: legacy\ndescription: legacy\n---\nbody")
	reg, diags := LoadFor(workspace)
	if reg.Len() != 0 || len(diags) != 0 {
		t.Fatalf("legacy .or root should not be scanned: skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestProjectOverridesUser(t *testing.T) {
	userRoot := t.TempDir()
	projectRoot := t.TempDir()
	writeSkill(t, userRoot, "commit", commitSkill)
	writeSkill(t, projectRoot, "commit", "---\nname: commit\ndescription: project-specific commit skill\n---\nproject body\n")

	reg, diags := Load(LoadOptions{UserDir: userRoot, ProjectDir: projectRoot})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	s, _ := reg.Lookup("commit")
	if s.Source != SourceProject || s.Description != "project-specific commit skill" || reg.Len() != 1 {
		t.Fatalf("resolved skill = %+v, registry length %d", s, reg.Len())
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
	if strings.Join(got, ",") != "commit,explain,review" {
		t.Errorf("List order = %v", got)
	}
}

func TestRegistrySnapshotsCopyMetadata(t *testing.T) {
	metadata := map[string]string{"version": "1"}
	reg := NewRegistry([]Skill{{Name: "review", Metadata: metadata}})
	metadata["version"] = "changed outside"

	listed := reg.List()
	listed[0].Metadata["version"] = "changed through list"
	lookedUp, ok := reg.Lookup("review")
	if !ok {
		t.Fatal("review not found")
	}
	lookedUp.Metadata["version"] = "changed through lookup"

	got, _ := reg.Lookup("review")
	if got.Metadata["version"] != "1" {
		t.Fatalf("registry metadata was mutated: %#v", got.Metadata)
	}
}

func TestNameMustMatchDirectory(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", "---\nname: review\ndescription: d\n---\nbody\n")
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 1 || !strings.Contains(diags[0].Message, "must match directory name") {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestMissingRequiredFieldsAreDiagnostics(t *testing.T) {
	for _, contents := range []string{
		"---\ndescription: d\n---\nbody",
		"---\nname: commit\n---\nbody",
		"# no frontmatter here\n",
	} {
		root := t.TempDir()
		writeSkill(t, root, "commit", contents)
		reg, diags := Load(LoadOptions{UserDir: root})
		if reg.Len() != 0 || len(diags) != 1 {
			t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
		}
	}
}

func TestDirectoryWithoutSkillFileIgnored(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, diags := Load(LoadOptions{UserDir: root})
	if reg.Len() != 0 || len(diags) != 0 {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestAbsentRootIsNotAnError(t *testing.T) {
	reg, diags := Load(LoadOptions{UserDir: filepath.Join(t.TempDir(), "does-not-exist")})
	if reg.Len() != 0 || len(diags) != 0 {
		t.Fatalf("skills = %d diagnostics = %+v", reg.Len(), diags)
	}
}

func TestToolLoadsSkillBodyUnchanged(t *testing.T) {
	root := t.TempDir()
	dir := writeSkill(t, root, "commit", commitSkill)
	reg, _ := Load(LoadOptions{UserDir: root})
	tool := reg.Tool()

	args, _ := json.Marshal(skillCallArgs{Name: "commit"})
	res, err := tool.Execute(t.Context(), "call-1", args, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	text := resultText(t, res)
	for _, want := range []string{
		`<loaded_skill name="commit"`,
		`root="` + dir + `"`,
		relativePathProtocol,
		"$ARGUMENTS and $1 remain literal skill instructions.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("tool result missing %q: %q", want, text)
		}
	}
}

func TestToolUnknownSkillReturnsError(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "commit", commitSkill)
	reg, _ := Load(LoadOptions{UserDir: root})
	args, _ := json.Marshal(skillCallArgs{Name: "nope"})
	res, err := reg.Tool().Execute(t.Context(), "call-1", args, nil)
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Unknown skill") || !strings.Contains(text, "commit") {
		t.Errorf("error should name valid skills, got %q", text)
	}
}

func TestExplicitInvocationLoadsBodyWithoutSubstitution(t *testing.T) {
	reg := NewRegistry([]Skill{{
		Name: "deploy", Description: "deploy the app",
		Content: "target=$ARGUMENTS first=$1 root=${OR_SKILL_DIR}", Dir: "/skills/deploy",
	}})
	loaded, matched, err := reg.ResolveExplicitInvocation("$deploy staging")
	if err != nil || !matched {
		t.Fatalf("matched = %v error = %v", matched, err)
	}
	for _, want := range []string{
		explicitInvocationPrefix,
		`name="deploy"`,
		`task details remain in the visible message: "staging"`,
		"target=$ARGUMENTS first=$1 root=${OR_SKILL_DIR}",
		relativePathProtocol,
	} {
		if !strings.Contains(loaded, want) {
			t.Errorf("loaded invocation missing %q:\n%s", want, loaded)
		}
	}
	if !IsExplicitInvocationText(loaded) {
		t.Fatal("loaded block was not recognizable by UI projection")
	}
}

func TestLegacySkillCommandIsNotRecognized(t *testing.T) {
	reg := NewRegistry([]Skill{{Name: "deploy", Description: "deploy", Content: "deploy"}})
	if _, matched, err := reg.ResolveExplicitInvocation("/skill:deploy staging"); err != nil || matched {
		t.Fatalf("legacy invocation matched = %v error = %v", matched, err)
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
		{input: "$deploy staging", want: "[$deploy](/skills/deploy/SKILL.md) staging"},
		{input: "  $review  ", want: "[$review](</skills/review path/SKILL.md>)"},
		{input: "[$deploy](/old/SKILL.md) staging", want: "[$deploy](/skills/deploy/SKILL.md) staging"},
		{input: "/review staging", want: "/review staging"},
	} {
		if got := registry.DisplayExplicitInvocation(tt.input); got != tt.want {
			t.Errorf("DisplayExplicitInvocation(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

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
