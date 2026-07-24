package transcript

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
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

func TestJSONLRejectsVersion2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	data := []byte("{\"type\":\"session\",\"version\":2}\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewJSONL(path)
	if _, err := store.Load(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "unsupported session version 2") {
		t.Fatalf("Load() error = %v, want unsupported version 2", err)
	}
}

func TestJSONLRoundTripsRunTiming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	startedAt := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	message := NewMessage(agent.UserMessage("hello"))
	run := NewRun(message.ID, startedAt, startedAt.Add(2*time.Second))
	store := NewJSONL(path)
	if err := store.Append(context.Background(), message, run); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"version":3`)) {
		t.Fatalf("session header is not v3:\n%s", data)
	}
	if bytes.Contains(data, []byte(`"parentId"`)) {
		t.Fatalf("linear session contains parentId:\n%s", data)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[1].Type != RunEntry || entries[1].Run == nil {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[1].Run.FirstEntryID != message.ID || !entries[1].Run.StartedAt.Equal(startedAt) {
		t.Fatalf("run timing = %#v", entries[1].Run)
	}
}

func TestJSONLRoundTripsContextAttachment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	entry := NewContext(ContextAttachment{
		AttachmentID: "session:2:abc",
		Epoch:        2,
		Kind:         "session",
		Placement:    "prefix",
		Revision:     "abc",
		Rendered:     `<or-context kind="session">rules</or-context>`,
	})
	store := NewJSONL(path)
	if err := store.Append(context.Background(), entry); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != ContextEntry || entries[0].Context == nil {
		t.Fatalf("entries = %#v", entries)
	}
	got := entries[0].Context
	if got.AttachmentID != entry.Context.AttachmentID ||
		got.Epoch != 2 ||
		got.Kind != "session" ||
		got.Placement != "prefix" ||
		got.Revision != "abc" ||
		got.Rendered != entry.Context.Rendered {
		t.Fatalf("context attachment = %#v", got)
	}
}
