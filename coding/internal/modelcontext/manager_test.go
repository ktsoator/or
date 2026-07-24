package modelcontext

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestPrepareStepPrependsBaseWithoutMutatingCanonicalInput(t *testing.T) {
	manager := New(3, `<or-context kind="session">rules</or-context>`)
	canonical := llm.Context{
		SystemPrompt: "stable",
		Messages:     []llm.Message{llm.UserText("question")},
		Tools:        []llm.ToolDefinition{{Name: "read"}},
	}

	prepared := manager.PrepareStep(canonical)
	if prepared.Input.SystemPrompt != "stable" {
		t.Fatalf("system prompt = %q, want stable", prepared.Input.SystemPrompt)
	}
	if len(prepared.Input.Messages) != 2 {
		t.Fatalf("projected messages = %d, want context and user", len(prepared.Input.Messages))
	}
	if got := userText(t, prepared.Input.Messages[0]); got != `<or-context kind="session">rules</or-context>` {
		t.Fatalf("base context = %q", got)
	}
	if got := userText(t, prepared.Input.Messages[1]); got != "question" {
		t.Fatalf("canonical user = %q", got)
	}
	if len(canonical.Messages) != 1 || userText(t, canonical.Messages[0]) != "question" {
		t.Fatal("PrepareStep mutated canonical messages")
	}
	if len(prepared.Pending) != 1 || prepared.Pending[0].Epoch != 3 {
		t.Fatalf("pending attachments = %#v", prepared.Pending)
	}
}

func TestCommitStopsRepersistingButKeepsProjectingBase(t *testing.T) {
	manager := New(1, "base")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}

	first := manager.PrepareStep(canonical)
	manager.Commit(first)
	second := manager.PrepareStep(canonical)

	if len(second.Pending) != 0 {
		t.Fatalf("pending after commit = %#v", second.Pending)
	}
	if len(second.Input.Messages) != 2 || userText(t, second.Input.Messages[0]) != "base" {
		t.Fatalf("base context stopped projecting after commit: %#v", second.Input.Messages)
	}
	state := manager.State()
	if !state.HasBase || !state.BaseCommitted || state.Epoch != 1 || state.BaseRevision == "" {
		t.Fatalf("state = %#v", state)
	}
}

func TestEmptyBaseLeavesInputUnchanged(t *testing.T) {
	manager := New(2, "")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}
	prepared := manager.PrepareStep(canonical)
	if len(prepared.Input.Messages) != 1 || len(prepared.Pending) != 0 {
		t.Fatalf("prepared = %#v", prepared)
	}
	state := manager.State()
	if state.HasBase || !state.BaseCommitted || state.Epoch != 2 {
		t.Fatalf("state = %#v", state)
	}
}

func userText(t *testing.T, message llm.Message) string {
	t.Helper()
	user, ok := message.(*llm.UserMessage)
	if !ok {
		t.Fatalf("message = %T, want user", message)
	}
	var text string
	for _, content := range user.Content {
		if block, ok := content.(*llm.TextContent); ok {
			text += block.Text
		}
	}
	return text
}
