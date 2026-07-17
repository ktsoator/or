package tools

import "testing"

func TestDiffLinesIdentical(t *testing.T) {
	hunks, add, del := diffLines("a\nb\nc\n", "a\nb\nc\n")
	if len(hunks) != 0 || add != 0 || del != 0 {
		t.Fatalf("identical inputs: got hunks=%d add=%d del=%d", len(hunks), add, del)
	}
}

func TestDiffLinesPureAddition(t *testing.T) {
	hunks, add, del := diffLines("", "x\ny\n")
	if add != 2 || del != 0 {
		t.Fatalf("got add=%d del=%d, want 2/0", add, del)
	}
	if len(hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(hunks))
	}
	if got := hunks[0].NewLines; got != 2 {
		t.Fatalf("NewLines=%d, want 2", got)
	}
	wantLines := []string{"+x", "+y"}
	if len(hunks[0].Lines) != len(wantLines) {
		t.Fatalf("lines=%v", hunks[0].Lines)
	}
	for i, w := range wantLines {
		if hunks[0].Lines[i] != w {
			t.Fatalf("line %d = %q, want %q", i, hunks[0].Lines[i], w)
		}
	}
}

func TestDiffLinesPureDeletion(t *testing.T) {
	_, add, del := diffLines("x\ny\nz\n", "")
	if add != 0 || del != 3 {
		t.Fatalf("got add=%d del=%d, want 0/3", add, del)
	}
}

func TestDiffLinesReplaceMiddleWithContext(t *testing.T) {
	old := "l1\nl2\nl3\nl4\nl5\n"
	new := "l1\nl2\nCHANGED\nl4\nl5\n"
	hunks, add, del := diffLines(old, new)
	if add != 1 || del != 1 {
		t.Fatalf("got add=%d del=%d, want 1/1", add, del)
	}
	if len(hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(hunks))
	}
	h := hunks[0]
	// Context should include l1..l5 (context=3 covers the whole small file).
	if h.OldStart != 1 || h.NewStart != 1 {
		t.Fatalf("starts old=%d new=%d, want 1/1", h.OldStart, h.NewStart)
	}
	found := false
	for _, l := range h.Lines {
		if l == "-l3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected -l3 in hunk lines: %v", h.Lines)
	}
}

func TestDiffLinesNoTrailingNewline(t *testing.T) {
	_, add, del := diffLines("a", "a\nb")
	if add != 1 || del != 0 {
		t.Fatalf("got add=%d del=%d, want 1/0", add, del)
	}
}

func TestDiffLinesSeparateHunks(t *testing.T) {
	// Two changes far apart should produce two hunks.
	old := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n"
	new := "1x\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15x\n"
	hunks, add, del := diffLines(old, new)
	if add != 2 || del != 2 {
		t.Fatalf("got add=%d del=%d, want 2/2", add, del)
	}
	if len(hunks) != 2 {
		t.Fatalf("got %d hunks, want 2", len(hunks))
	}
}
