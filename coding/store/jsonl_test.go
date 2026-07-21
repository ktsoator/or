package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestJSONLLoadRejectsLegacyMessagesWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	legacy := []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hello"}}},
		&llm.AssistantMessage{Content: []llm.AssistantContent{&llm.TextContent{Text: "world"}}, StopReason: llm.StopReasonStop},
	}
	var data bytes.Buffer
	for _, message := range legacy {
		line, err := llm.MarshalMessage(message)
		if err != nil {
			t.Fatal(err)
		}
		data.Write(line)
		data.WriteByte('\n')
	}
	if err := os.WriteFile(path, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewJSONL(path)
	if _, err := store.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid session header") {
		t.Fatalf("Load() error = %v, want invalid session header", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data.Bytes()) {
		t.Fatal("legacy session was rewritten")
	}
}
