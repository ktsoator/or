package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/agent"
)

// TestJSONLCreatesMissingDirs checks that Append and Replace create the session
// file's parent directories, so a fresh workspace persists without a manual
// mkdir. This guards the regression where the first turn failed on a missing
// .coding directory.
func TestJSONLCreatesMissingDirs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "dir", "session.jsonl")
	s := NewJSONL(path)

	msg := agent.UserMessage("hello")
	if err := s.Append(ctx, msg); err != nil {
		t.Fatalf("append into missing dir: %v", err)
	}

	loaded, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("want 1 message, got %d", len(loaded))
	}

	// Replace into a different missing tree must also create it.
	path2 := filepath.Join(t.TempDir(), "a", "b", "session.jsonl")
	s2 := NewJSONL(path2)
	if err := s2.Replace(ctx, []agent.AgentMessage{msg}); err != nil {
		t.Fatalf("replace into missing dir: %v", err)
	}
}
