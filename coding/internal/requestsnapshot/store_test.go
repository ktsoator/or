package requestsnapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestFileStoreRoundTripsPrivateSanitizedSnapshot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "diagnostics", "requests")
	store, err := NewFileStore(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	input := llm.Context{
		SystemPrompt: "system instructions\nAuthorization: Bearer private-bearer-token",
		Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{
				&llm.TextContent{Text: "question", TextSignature: "private-text-signature"},
				&llm.ImageContent{Data: "aGVsbG8=", MIMEType: "image/png"},
			}},
			&llm.AssistantMessage{ProviderRequestID: "request-previous", Content: []llm.AssistantContent{
				&llm.ThinkingContent{Thinking: "hidden", ThinkingSignature: "private-thinking-signature", Redacted: true},
				&llm.ToolCall{ID: "call-1", Name: "shell", ThoughtSignature: "private-thought-signature", Arguments: map[string]any{
					"command": "pwd", "token_budget": 2048, "api_key": "private-api-key", "nested": map[string]any{"accessToken": "private-token"},
				}},
			}},
		},
		Tools: []llm.ToolDefinition{{
			Name: "shell", Description: "Run a command with sk-privateCredential123",
			Parameters: json.RawMessage(`{"type":"object","default":{"api_key":"private-schema-key"}}`),
		}},
	}
	snapshot := NewSnapshot(
		"session-1", "run-1", "turn-1", "request-1", "test", "model", input,
		[]Attachment{{ID: "context-1", Kind: "base", Placement: "prefix", MessageIndex: 0}},
	)
	if err := store.Save(snapshot); err != nil {
		t.Fatal(err)
	}
	output := &llm.AssistantMessage{
		StopReason: llm.StopReasonStop,
		Content: []llm.AssistantContent{
			&llm.TextContent{Text: "answer with api_key=private-output-key"},
		},
	}
	if err := store.SaveOutput("request-1", output); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("request-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Input.SystemPrompt != "system instructions\nAuthorization: Bearer [redacted]" || len(loaded.Input.Messages) != 2 || len(loaded.Input.Tools) != 1 {
		t.Fatalf("loaded snapshot = %#v", loaded)
	}
	if loaded.Output == nil || loaded.Output.Message.Content[0].Text != "answer with api_key=[redacted]" || loaded.Output.StopReason != "stop" {
		t.Fatalf("loaded output = %#v", loaded.Output)
	}
	if loaded.Input.Messages[1].ProviderRequestID != "request-previous" ||
		loaded.Output.Message.ProviderRequestID != "request-1" {
		t.Fatalf("provider request provenance = input %q, output %q",
			loaded.Input.Messages[1].ProviderRequestID, loaded.Output.Message.ProviderRequestID)
	}
	image := loaded.Input.Messages[0].Content[1].Image
	if image == nil || image.MIMEType != "image/png" || image.EncodedBytes != 5 {
		t.Fatalf("image metadata = %#v", image)
	}
	thinking := loaded.Input.Messages[1].Content[0]
	if !thinking.Redacted || thinking.Thinking != "[redacted reasoning omitted]" {
		t.Fatalf("thinking = %#v", thinking)
	}
	arguments := loaded.Input.Messages[1].Content[1].Arguments
	if arguments["command"] != "pwd" || arguments["api_key"] != "[redacted]" {
		t.Fatalf("arguments = %#v", arguments)
	}
	if arguments["token_budget"] != float64(2048) && arguments["token_budget"] != 2048 {
		t.Fatalf("non-secret token budget was redacted: %#v", arguments)
	}
	if nested := arguments["nested"].(map[string]any); nested["accessToken"] != "[redacted]" {
		t.Fatalf("nested arguments = %#v", nested)
	}

	path := filepath.Join(dir, "request-1.json")
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"aGVsbG8=", "private-text-signature", "private-thinking-signature",
		"private-thought-signature", "private-api-key", "private-token",
		"private-bearer-token", "privateCredential123", "private-schema-key",
		"private-output-key",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("snapshot contains %q: %s", forbidden, encoded)
		}
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}
	if info, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %o, want 700", info.Mode().Perm())
	}
}

func TestFileStorePrunesOldestSnapshots(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), Options{MaxSnapshots: 2, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, requestID := range []string{"request-1", "request-2", "request-3"} {
		if err := store.Save(NewSnapshot("session", "run", "turn", requestID, "test", "model", llm.Context{}, nil)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Load("request-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest load error = %v, want not found", err)
	}
	for _, requestID := range []string{"request-2", "request-3"} {
		if _, err := store.Load(requestID); err != nil {
			t.Fatalf("load %s: %v", requestID, err)
		}
	}
}

func TestFileStoreDeletesSnapshotsForOneSession(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range []Snapshot{
		NewSnapshot("session-1", "run-1", "turn-1", "request-1", "test", "model", llm.Context{}, nil),
		NewSnapshot("session-1", "run-2", "turn-2", "request-2", "test", "model", llm.Context{}, nil),
		NewSnapshot("session-2", "run-3", "turn-3", "request-3", "test", "model", llm.Context{}, nil),
	} {
		if err := store.Save(snapshot); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.DeleteSession("session-1"); err != nil {
		t.Fatal(err)
	}
	for _, requestID := range []string{"request-1", "request-2"} {
		if _, err := store.Load(requestID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("load %s error = %v, want not found", requestID, err)
		}
	}
	if _, err := store.Load("request-3"); err != nil {
		t.Fatalf("preserved snapshot: %v", err)
	}
}

func TestFileStoreRejectsInvalidRequestID(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("../escape"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("load error = %v, want invalid ID", err)
	}
}
