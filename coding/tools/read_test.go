package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReadTextRange(t *testing.T) {
	result, err := readTextRange(context.Background(), strings.NewReader("one\ntwo\nthree\nfour\n"), 2, 2, DefaultMaxBytes-readNoticeBudget)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "two\nthree" {
		t.Fatalf("content = %q", result.Content)
	}
	if result.StartLine != 2 || result.LineCount != 2 || !result.HasMore || result.NextOffset != 4 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := formatReadResult(result); !strings.Contains(got, "use offset and limit to read any other range") {
		t.Fatalf("partial-view notice = %q", got)
	}
}

func TestFormatReadResultMarksEarlierRangeAsPartial(t *testing.T) {
	result, err := readTextRange(context.Background(), strings.NewReader("one\ntwo\nthree\n"), 2, 10, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasMore {
		t.Fatalf("expected no later content: %+v", result)
	}
	if got := formatReadResult(result); !strings.Contains(got, "This is a partial view") {
		t.Fatalf("partial-view notice = %q", got)
	}
}

func TestReadTextRangeStopsAtByteLimit(t *testing.T) {
	result, err := readTextRange(context.Background(), strings.NewReader("12345\n67890\nlast\n"), 1, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "12345" || !result.HasMore || result.NextOffset != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestReadTextRangeRejectsOversizedLine(t *testing.T) {
	_, err := readTextRange(context.Background(), strings.NewReader(strings.Repeat("x", DefaultMaxBytes+1)), 1, 1, DefaultMaxBytes-readNoticeBudget)
	if err == nil || !strings.Contains(err.Error(), "line 1 exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadTextRangeEmptyAndOffsetPastEnd(t *testing.T) {
	empty, err := readTextRange(context.Background(), strings.NewReader(""), 1, 10, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if empty.LineCount != 0 || formatReadResult(empty) != "(empty file)" {
		t.Fatalf("unexpected empty result: %+v", empty)
	}

	pastEnd, err := readTextRange(context.Background(), strings.NewReader("one\ntwo\n"), 5, 10, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if got := formatReadResult(pastEnd); got != "[No content at or after line 5.]" {
		t.Fatalf("message = %q", got)
	}
}

func TestReadTextRangeHandlesLongLinesWithoutScannerLimit(t *testing.T) {
	longLine := strings.Repeat("x", 128*1024)
	result, err := readTextRange(context.Background(), strings.NewReader(longLine+"\ntarget\n"), 2, 1, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "target" || result.LineCount != 1 || result.HasMore {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestReadTextRangeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readTextRange(ctx, strings.NewReader("one\n"), 1, 1, DefaultMaxBytes)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeReadArgs(t *testing.T) {
	offset, limit, err := normalizeReadArgs(readArgs{Path: "file.go"})
	if err != nil {
		t.Fatal(err)
	}
	if offset != 1 || limit != DefaultMaxLines {
		t.Fatalf("offset=%d limit=%d", offset, limit)
	}

	invalid := []readArgs{
		{},
		{Path: "file.go", Offset: -1},
		{Path: "file.go", Limit: -1},
		{Path: "file.go", Limit: maxReadLines + 1},
	}
	for _, in := range invalid {
		if _, _, err := normalizeReadArgs(in); err == nil {
			t.Fatalf("expected error for %+v", in)
		}
	}
}
