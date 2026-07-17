package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/agent"
)

func runTool(t *testing.T, tool Tool, args any) (agent.ToolResult, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return tool.AgentTool.Execute(context.Background(), "call", raw, func(agent.ToolResult) {})
}

// record reads a file through the Read tool so the file-state store observes it,
// mirroring the read-before-edit flow the real agent follows.
func record(t *testing.T, root string, files *FileStateStore, rel string) {
	t.Helper()
	if _, err := runTool(t, Read(root, LocalOps{}, files), map[string]any{"path": rel}); err != nil {
		t.Fatalf("record read %s: %v", rel, err)
	}
}

func TestWriteCreateReportsChange(t *testing.T) {
	root := t.TempDir()
	files := NewFileStateStore()
	res, err := runTool(t, Write(root, LocalOps{}, files), map[string]any{"path": "new.txt", "content": "a\nb\nc\n"})
	if err != nil {
		t.Fatal(err)
	}
	change, ok := res.Details.(FileChange)
	if !ok {
		t.Fatalf("Details = %T, want FileChange", res.Details)
	}
	if change.Kind != ChangeCreate {
		t.Fatalf("Kind = %q, want create", change.Kind)
	}
	if change.Additions != 3 || change.Deletions != 0 {
		t.Fatalf("+%d -%d, want +3 -0", change.Additions, change.Deletions)
	}
}

func TestWriteUpdateReportsDiff(t *testing.T) {
	root := t.TempDir()
	files := NewFileStateStore()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record(t, root, files, "f.txt")

	res, err := runTool(t, Write(root, LocalOps{}, files), map[string]any{"path": "f.txt", "content": "one\nTWO\nthree\n"})
	if err != nil {
		t.Fatal(err)
	}
	change, ok := res.Details.(FileChange)
	if !ok {
		t.Fatalf("Details = %T, want FileChange", res.Details)
	}
	if change.Kind != ChangeUpdate {
		t.Fatalf("Kind = %q, want update", change.Kind)
	}
	if change.Additions != 1 || change.Deletions != 1 {
		t.Fatalf("+%d -%d, want +1 -1", change.Additions, change.Deletions)
	}
	if len(change.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(change.Hunks))
	}
}

func TestEditReportsChange(t *testing.T) {
	root := t.TempDir()
	files := NewFileStateStore()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record(t, root, files, "f.txt")

	res, err := runTool(t, Edit(root, LocalOps{}, files), map[string]any{"path": "f.txt", "old_string": "world", "new_string": "mars"})
	if err != nil {
		t.Fatal(err)
	}
	change, ok := res.Details.(FileChange)
	if !ok {
		t.Fatalf("Details = %T, want FileChange", res.Details)
	}
	if change.Kind != ChangeUpdate || change.Additions != 1 || change.Deletions != 1 {
		t.Fatalf("unexpected change: %+v", change)
	}
}

func TestEditWithoutReadReportsFailure(t *testing.T) {
	root := t.TempDir()
	files := NewFileStateStore()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Deliberately skip the read-before-edit step.
	res, err := runTool(t, Edit(root, LocalOps{}, files), map[string]any{"path": "f.txt", "old_string": "hello", "new_string": "bye"})
	if err == nil {
		t.Fatal("expected an error")
	}
	fail, ok := res.Details.(MutationFailure)
	if !ok {
		t.Fatalf("Details = %T, want MutationFailure", res.Details)
	}
	if fail.Reason != FailureNotRead {
		t.Fatalf("Reason = %q, want %q", fail.Reason, FailureNotRead)
	}
}

func TestEditAmbiguousReportsFailure(t *testing.T) {
	root := t.TempDir()
	files := NewFileStateStore()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record(t, root, files, "f.txt")

	res, err := runTool(t, Edit(root, LocalOps{}, files), map[string]any{"path": "f.txt", "old_string": "x", "new_string": "y"})
	if err == nil {
		t.Fatal("expected an error")
	}
	fail, ok := res.Details.(MutationFailure)
	if !ok {
		t.Fatalf("Details = %T, want MutationFailure", res.Details)
	}
	if fail.Reason != FailureAmbiguous {
		t.Fatalf("Reason = %q, want %q", fail.Reason, FailureAmbiguous)
	}
}
