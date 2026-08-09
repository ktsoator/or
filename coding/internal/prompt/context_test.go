package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadContextFilesOrdersEveryScopeBroadestFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".or", "AGENTS.md"), "user rule")

	parent := t.TempDir()
	workspace := filepath.Join(parent, "service")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(parent, "AGENTS.md"), "repo rule")
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "service rule")
	writeFile(t, filepath.Join(workspace, "AGENTS.local.md"), "local rule")

	files := LoadContextFiles(workspace)
	got := make([]ContextFile, 0, len(files))
	for _, file := range files {
		// Ancestors above the temp roots belong to the host, not the fixture.
		switch file.Content {
		case "user rule", "repo rule", "service rule", "local rule":
			got = append(got, file)
		}
	}

	want := []struct {
		content string
		scope   ContextScope
	}{
		{"user rule", ScopeUser},
		{"repo rule", ScopeProject},
		{"service rule", ScopeProject},
		{"local rule", ScopeLocal},
	}
	if len(got) != len(want) {
		t.Fatalf("discovered files = %#v, want %d layers", got, len(want))
	}
	for index, expected := range want {
		if got[index].Content != expected.content || got[index].Scope != expected.scope {
			t.Errorf(
				"file %d = %q/%q, want %q/%q",
				index,
				got[index].Content,
				got[index].Scope,
				expected.content,
				expected.scope,
			)
		}
	}
}

func TestLoadContextFilesIgnoresClaudeFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	claude := filepath.Join(workspace, "CLAUDE.md")
	localClaude := filepath.Join(workspace, "CLAUDE.local.md")
	writeFile(t, claude, "unsupported")
	writeFile(t, localClaude, "unsupported local")

	for _, file := range LoadContextFiles(workspace) {
		if file.Path == claude || file.Path == localClaude {
			t.Fatalf("unsupported context file was loaded: %s", file.Path)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
