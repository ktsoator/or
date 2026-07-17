package coding

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ktsoator/or/coding/store"
	"github.com/ktsoator/or/coding/tools"
)

func TestEncodeDecodeFileChangeRoundTrip(t *testing.T) {
	change := tools.FileChange{
		Path:      "src/foo.go",
		Kind:      tools.ChangeUpdate,
		Additions: 2,
		Deletions: 1,
		Bytes:     120,
		Hunks: []tools.Hunk{{
			OldStart: 1, OldLines: 4, NewStart: 1, NewLines: 5,
			Lines: []string{" package main", "-func old() {}", "+func neu() {}"},
		}},
	}
	raw, ok := encodeDetails(change)
	if !ok {
		t.Fatal("encodeDetails returned ok=false")
	}
	got, ok := decodeDetails(raw).(tools.FileChange)
	if !ok {
		t.Fatalf("decoded type = %T, want tools.FileChange", decodeDetails(raw))
	}
	if !reflect.DeepEqual(got, change) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, change)
	}
}

func TestEncodeDecodeMutationFailureRoundTrip(t *testing.T) {
	fail := tools.MutationFailure{Path: "a.go", Reason: tools.FailureNotRead, Detail: "not read"}
	raw, ok := encodeDetails(fail)
	if !ok {
		t.Fatal("encodeDetails returned ok=false")
	}
	got, ok := decodeDetails(raw).(tools.MutationFailure)
	if !ok || got != fail {
		t.Fatalf("round trip mismatch: %+v", decodeDetails(raw))
	}
}

func TestEncodeDetailsSkipsUnknown(t *testing.T) {
	if _, ok := encodeDetails("just a string"); ok {
		t.Fatal("expected ok=false for an unrecognized value")
	}
}

func TestDetailsSurviveStoreReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "s.details.jsonl")
	change := tools.FileChange{Path: "x.go", Kind: tools.ChangeCreate, Additions: 3}

	raw, ok := encodeDetails(change)
	if !ok {
		t.Fatal("encode failed")
	}
	if err := store.NewJSONLDetails(path).Put(ctx, "call-1", raw); err != nil {
		t.Fatal(err)
	}

	// A fresh store instance simulates a reloaded process.
	loaded, err := store.NewJSONLDetails(path).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decodeDetails(loaded["call-1"]).(tools.FileChange)
	if !ok || !reflect.DeepEqual(got, change) {
		t.Fatalf("reloaded details mismatch: %+v", decodeDetails(loaded["call-1"]))
	}
}

func TestDetailsStoreLastWriteWins(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "s.details.jsonl")
	ds := store.NewJSONLDetails(path)

	first, _ := encodeDetails(tools.FileChange{Path: "x.go", Additions: 1})
	second, _ := encodeDetails(tools.FileChange{Path: "x.go", Additions: 9})
	if err := ds.Put(ctx, "call-1", first); err != nil {
		t.Fatal(err)
	}
	if err := ds.Put(ctx, "call-1", second); err != nil {
		t.Fatal(err)
	}

	loaded, err := ds.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := decodeDetails(loaded["call-1"]).(tools.FileChange)
	if got.Additions != 9 {
		t.Fatalf("Additions = %d, want 9 (last write should win)", got.Additions)
	}
}
