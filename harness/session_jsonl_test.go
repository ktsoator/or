package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/harness"
)

func TestJSONLSessionRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	session := harness.NewJSONLSession(path)

	// A missing file loads as empty history.
	if loaded, err := session.Load(ctx); err != nil || len(loaded) != 0 {
		t.Fatalf("Load() on missing file = %v, %v; want empty, nil", loaded, err)
	}

	want := []agent.AgentMessage{userMsg("hello"), assistantMsg("hi there")}
	if err := session.Append(ctx, want...); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	// A fresh session over the same path reads what was written, across instances.
	reopened := harness.NewJSONLSession(path)
	got, err := reopened.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d messages, want 2", len(got))
	}
	if text := messageText(t, got[0]); text != "hello" {
		t.Fatalf("first message text = %q, want %q", text, "hello")
	}
}

func TestJSONLSessionAppendIsCumulative(t *testing.T) {
	ctx := context.Background()
	session := harness.NewJSONLSession(filepath.Join(t.TempDir(), "session.jsonl"))

	if err := session.Append(ctx, userMsg("one")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := session.Append(ctx, assistantMsg("two")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	got, err := session.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d messages, want 2 (appends accumulate)", len(got))
	}
}

func TestJSONLSessionReplaceRewrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	session := harness.NewJSONLSession(path)

	if err := session.Append(ctx, userMsg("a"), assistantMsg("b"), userMsg("c")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := session.Replace(ctx, []agent.AgentMessage{userMsg("summary")}); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	got, err := session.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 1 || messageText(t, got[0]) != "summary" {
		t.Fatalf("after Replace, transcript = %d msgs, want [summary]", len(got))
	}
	// No leftover temp file.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file lingered: %v", err)
	}
}

func TestJSONLSessionResumesThroughHarness(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.jsonl")

	first, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: scriptedStream("reply one"),
		Session:  harness.NewJSONLSession(path),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := first.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	// A new harness over a new session at the same path resumes the transcript.
	second, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: scriptedStream("reply two"),
		Session:  harness.NewJSONLSession(path),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := len(second.Snapshot().Messages); got != 2 {
		t.Fatalf("resumed transcript has %d messages, want 2", got)
	}
}

func TestJSONLSessionRejectsCustomMessage(t *testing.T) {
	ctx := context.Background()
	session := harness.NewJSONLSession(filepath.Join(t.TempDir(), "session.jsonl"))
	if err := session.Append(ctx, uiNote{}); err == nil {
		t.Fatal("Append(custom message) = nil, want error")
	}
}

// uiNote is a custom UI-only message with no llm projection.
type uiNote struct{ agent.Custom }
