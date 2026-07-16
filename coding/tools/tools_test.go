package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// run invokes a tool's Execute with JSON-marshaled args and returns the text of
// its result plus the returned error.
func run(t *testing.T, tool Tool, args any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	result, execErr := tool.Execute(context.Background(), "call-1", raw, func(agent.ToolResult) {})
	return resultText(result), execErr
}

// resultText concatenates the text blocks of a tool result.
func resultText(result agent.ToolResult) string {
	var parts []string
	for _, c := range result.Content {
		if text, ok := c.(*llm.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func TestReadWriteEditRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ops := LocalOps{}

	// Write a new file.
	write := Write(dir, ops)
	if _, err := run(t, write, writeArgs{Path: "hello.txt", Content: "line one\nline two\n"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "hello.txt")); got != "line one\nline two\n" {
		t.Fatalf("write produced %q", got)
	}

	// Read it back with line numbers.
	read := Read(dir, ops)
	out, err := run(t, read, readArgs{Path: "hello.txt"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "1\tline one") || !strings.Contains(out, "2\tline two") {
		t.Fatalf("read output missing numbered lines: %q", out)
	}

	// Edit a unique substring.
	edit := Edit(dir, ops)
	if _, err := run(t, edit, editArgs{Path: "hello.txt", OldString: "line one", NewString: "line 1"}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "hello.txt")); got != "line 1\nline two\n" {
		t.Fatalf("edit produced %q", got)
	}
}

func TestEditRejectsAmbiguousMatch(t *testing.T) {
	dir := t.TempDir()
	ops := LocalOps{}
	if err := os.WriteFile(filepath.Join(dir, "dup.txt"), []byte("x\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit := Edit(dir, ops)

	// Two matches without replace_all must fail.
	if _, err := run(t, edit, editArgs{Path: "dup.txt", OldString: "x", NewString: "y"}); err == nil {
		t.Fatal("expected ambiguous edit to fail")
	}
	// replace_all succeeds.
	if _, err := run(t, edit, editArgs{Path: "dup.txt", OldString: "x", NewString: "y", ReplaceAll: true}); err != nil {
		t.Fatalf("replace_all: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "dup.txt")); got != "y\ny\n" {
		t.Fatalf("replace_all produced %q", got)
	}
}

func TestEditMissingStringFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	edit := Edit(dir, LocalOps{})
	if _, err := run(t, edit, editArgs{Path: "f.txt", OldString: "zzz", NewString: "q"}); err == nil {
		t.Fatal("expected missing old_string to fail")
	}
}

func TestBashRunsAndReportsExit(t *testing.T) {
	dir := t.TempDir()
	bash := Bash(dir, LocalOps{})

	out, err := run(t, bash, bashArgs{Command: "echo hi"})
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("bash output %q missing echo", out)
	}

	out, err = run(t, bash, bashArgs{Command: "exit 3"})
	if err != nil {
		t.Fatalf("bash non-zero exit should not be a Go error: %v", err)
	}
	if !strings.Contains(out, "exit code: 3") {
		t.Fatalf("bash output %q missing exit code", out)
	}
}

func TestTruncate(t *testing.T) {
	in := strings.Repeat("a\n", 100)
	out := truncate(in, 10, 0)
	if strings.Count(out, "\n") > 12 {
		t.Fatalf("line truncation kept too many lines: %d", strings.Count(out, "\n"))
	}
	if !strings.Contains(out, "truncated") {
		t.Fatal("expected truncation notice")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
