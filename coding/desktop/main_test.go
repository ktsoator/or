package main

import (
	"path/filepath"
	"testing"
)

func TestFirstExistingDirectory(t *testing.T) {
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")

	if got := firstExistingDirectory("", missing, existing); got != existing {
		t.Fatalf("firstExistingDirectory() = %q, want %q", got, existing)
	}
	if got := firstExistingDirectory("", missing); got != "" {
		t.Fatalf("firstExistingDirectory() = %q, want empty", got)
	}
}
