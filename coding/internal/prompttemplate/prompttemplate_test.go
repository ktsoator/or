package prompttemplate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemplate(t *testing.T, root, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadProjectOverridesUserAndParsesMetadata(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()
	writeTemplate(t, userDir, "review", "---\ndescription: User review\nargument-hint: '[focus]'\n---\nUser $ARGUMENTS")
	writeTemplate(t, userDir, "commit", "Write a commit message")
	writeTemplate(t, projectDir, "review", "---\ndescription: Project review\n---\nProject $1")

	registry, diagnostics := Load(LoadOptions{UserDir: userDir, ProjectDir: projectDir})
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	items := registry.List()
	if len(items) != 2 {
		t.Fatalf("templates = %+v, want two", items)
	}
	review, ok := registry.Lookup("review")
	if !ok || review.Source != SourceProject || review.Description != "Project review" {
		t.Fatalf("review = %+v, want project override", review)
	}
	commit, _ := registry.Lookup("commit")
	if commit.Description != "Write a commit message" {
		t.Fatalf("fallback description = %q", commit.Description)
	}
}

func TestLoadParsesLocalizedMetadata(t *testing.T) {
	root := t.TempDir()
	writeTemplate(t, root, "review", "---\ndescription-en: Review changes\ndescription-zh-CN: 审查改动\nargument-hint-en: '[focus]'\nargument-hint-zh-CN: '[关注点]'\n---\nReview")

	registry, diagnostics := Load(LoadOptions{UserDir: root})
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	template, ok := registry.Lookup("review")
	if !ok {
		t.Fatal("review template not loaded")
	}
	if template.Description != "Review changes" || template.Descriptions["zh-CN"] != "审查改动" {
		t.Fatalf("descriptions = %q / %+v", template.Description, template.Descriptions)
	}
	if template.ArgumentHint != "[focus]" || template.ArgumentHints["zh-CN"] != "[关注点]" {
		t.Fatalf("argument hints = %q / %+v", template.ArgumentHint, template.ArgumentHints)
	}
}

func TestLoadReportsMalformedFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeTemplate(t, root, "broken", "---\ndescription: [\n---\nbody")
	registry, diagnostics := Load(LoadOptions{UserDir: root})
	if len(registry.List()) != 0 || len(diagnostics) != 1 {
		t.Fatalf("registry = %+v diagnostics = %+v", registry.List(), diagnostics)
	}
}

func TestSubstituteSupportsQuotesDefaultsAndSlices(t *testing.T) {
	args := ParseArguments(`Button "click handler" disabled`)
	got := Substitute(`$1|$2|$ARGUMENTS|${4:-fallback}|${@:2}|${@:2:1}`, args)
	want := "Button|click handler|Button click handler disabled|fallback|click handler disabled|click handler"
	if got != want {
		t.Fatalf("substitution = %q, want %q", got, want)
	}
}

func TestExpansionAddsHiddenContextAndLeavesUnknownCommandsAlone(t *testing.T) {
	registry := NewRegistry([]Template{{
		Name: "review", Content: "Review $1", Path: "/prompts/review.md", Source: SourceProject,
	}})
	expanded, matched := registry.ExpandExplicitInvocation("/review security")
	if !matched || !IsExplicitInvocationText(expanded) || !strings.Contains(expanded, "Review security") {
		t.Fatalf("expanded = %q matched = %v", expanded, matched)
	}
	invoked, ok := ParseExplicitInvocationText(expanded)
	if !ok || invoked.Name != "review" || invoked.Source != string(SourceProject) ||
		invoked.Path != "/prompts/review.md" {
		t.Fatalf("invocation = %#v matched = %v", invoked, ok)
	}
	if _, matched := registry.ExpandExplicitInvocation("/missing args"); matched {
		t.Fatal("unknown command should remain ordinary text")
	}
}
