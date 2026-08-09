package transcript

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLDetailsUsesPrivatePermissionsAndSecuresExistingStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	path := filepath.Join(dir, "session.details.jsonl")
	store := NewJSONLDetails(path)
	payload := json.RawMessage(`{"status":"success","data":{"secret":true}}`)
	if err := store.Put(context.Background(), "call-1", payload); err != nil {
		t.Fatal(err)
	}
	assertPrivateStoragePermissions(t, dir, path)

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewJSONLDetails(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(loaded["call-1"]); got != string(payload) {
		t.Fatalf("payload = %s, want %s", got, payload)
	}
	assertPrivateStoragePermissions(t, dir, path)
}
