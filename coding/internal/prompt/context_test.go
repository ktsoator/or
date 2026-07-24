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

func TestLoadContextFilesTakesOneFilePerDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "preferred")
	writeFile(t, filepath.Join(workspace, "CLAUDE.md"), "fallback")

	for _, file := range LoadContextFiles(workspace) {
		if file.Content == "fallback" {
			t.Fatal("CLAUDE.md was loaded alongside AGENTS.md in the same directory")
		}
	}

	other := t.TempDir()
	writeFile(t, filepath.Join(other, "CLAUDE.md"), "fallback")
	var found bool
	for _, file := range LoadContextFiles(other) {
		if file.Content == "fallback" && file.Scope == ScopeProject {
			found = true
		}
	}
	if !found {
		t.Fatal("CLAUDE.md was not used as the fallback name")
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
