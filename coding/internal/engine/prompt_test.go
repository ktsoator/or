package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestSessionProjectsBaseContextOutsideStableSystemPrompt(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspace, "AGENTS.md"),
		[]byte("Follow the workspace rule."),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	store := &checkpointStore{}
	var captured llm.Context
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   workspace,
		Tools: []tools.Tool{},
		Store: store,
		Skills: []skills.Skill{{
			Name:        "review",
			Description: "Review changes before completion",
			Content:     "Review the complete diff.",
			Dir:         filepath.Join(workspace, ".or", "skills", "review"),
		}},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			captured = input
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	system := session.agent.Snapshot().SystemPrompt
	for _, want := range []string{
		"## Project context protocol",
		"## Skills",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("session prompt missing %q:\n%s", want, system)
		}
	}
	for _, dynamic := range []string{
		`<or-context kind="base">`,
		`<or-context kind="skill_listing">`,
		"Follow the workspace rule.",
		"<name>review</name>",
	} {
		if strings.Contains(system, dynamic) {
			t.Errorf("stable system prompt contains dynamic value %q:\n%s", dynamic, system)
		}
	}

	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if captured.SystemPrompt != system {
		t.Fatalf("provider system prompt changed:\n%s", captured.SystemPrompt)
	}
	if len(captured.Messages) != 3 {
		t.Fatalf("provider messages = %d, want base, skill listing, and user", len(captured.Messages))
	}
	base := llmUserText(t, captured.Messages[0])
	for _, want := range []string{
		`<or-context kind="base">`,
		"Follow the workspace rule.",
	} {
		if !strings.Contains(base, want) {
			t.Errorf("Base Context missing %q:\n%s", want, base)
		}
	}
	listing := llmUserText(t, captured.Messages[1])
	for _, want := range []string{
		`<or-context kind="skill_listing"`,
		"<name>review</name>",
		"<description>Review changes before completion</description>",
	} {
		if !strings.Contains(listing, want) {
			t.Errorf("skill listing missing %q:\n%s", want, listing)
		}
	}
	if got := llmUserText(t, captured.Messages[2]); got != "question" {
		t.Fatalf("canonical user = %q, want question", got)
	}

	entries, batches, _ := store.snapshot()
	if len(entries) != 5 {
		t.Fatalf("durable entries = %d, want two contexts, user, assistant, run", len(entries))
	}
	if entries[0].Type != transcript.ContextEntry || entries[0].Context == nil {
		t.Fatalf("first durable entry = %#v, want hidden context", entries[0])
	}
	if entries[0].Context.Epoch != 1 ||
		entries[0].Context.Kind != "base" ||
		entries[0].Context.Placement != "prefix" {
		t.Fatalf("context metadata = %#v", entries[0].Context)
	}
	if entries[1].Type != transcript.ContextEntry ||
		entries[1].Context == nil ||
		entries[1].Context.Kind != "skill_listing" {
		t.Fatalf("second durable entry = %#v, want skill listing", entries[1])
	}
	if len(batches) != 2 || len(batches[0]) != 3 || len(batches[1]) != 2 {
		t.Fatalf("append batch sizes = %v, want [3 2]", batchSizes(batches))
	}
	history := session.History()
	for _, item := range history {
		if strings.Contains(item.Text, "<or-context") ||
			strings.Contains(item.Text, "Follow the workspace rule.") {
			t.Fatalf("hidden Base Context leaked into history: %#v", item)
		}
	}
}

func TestRestoredSessionStartsNewBaseContextEpochFromCurrentFiles(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("old rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &checkpointStore{}
	first, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   workspace,
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			return assistantEvents(model, "first"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Prompt(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, []byte("new rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured llm.Context
	restored, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   workspace,
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			captured = input
			return assistantEvents(model, "second"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Prompt(ctx, "two"); err != nil {
		t.Fatal(err)
	}

	if len(captured.Messages) != 4 {
		t.Fatalf("restored provider messages = %d, want context plus three canonical messages", len(captured.Messages))
	}
	base := llmUserText(t, captured.Messages[0])
	if !strings.Contains(base, "new rule") || strings.Contains(base, "old rule") {
		t.Fatalf("restored Base Context did not reload current files:\n%s", base)
	}
	entries, _, _ := store.snapshot()
	var contexts []*transcript.ContextAttachment
	for _, entry := range entries {
		if entry.Type == transcript.ContextEntry {
			contexts = append(contexts, entry.Context)
		}
	}
	if len(contexts) != 2 || contexts[0].Epoch != 1 || contexts[1].Epoch != 2 {
		t.Fatalf("context epochs = %#v", contexts)
	}
}

func TestBaseContextIsCheckpointedOnceAcrossAppRetry(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(workspace, "AGENTS.md"),
		[]byte("stable retry rule"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	store := &checkpointStore{}
	var inputs []llm.Context
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   workspace,
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			inputs = append(inputs, input)
			if len(inputs) == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonError
				message.ErrorMessage = "temporarily unavailable"
				return finalEvents(llm.EventError, &message), nil
			}
			return assistantEvents(model, "recovered"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(ctx, "retry this"); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(inputs))
	}
	firstBase := llmUserText(t, inputs[0].Messages[0])
	secondBase := llmUserText(t, inputs[1].Messages[0])
	if firstBase != secondBase || !strings.Contains(firstBase, "stable retry rule") {
		t.Fatalf("retry Base Context changed:\nfirst:\n%s\nsecond:\n%s", firstBase, secondBase)
	}

	entries, batches, _ := store.snapshot()
	var contextCount int
	for _, entry := range entries {
		if entry.Type == transcript.ContextEntry {
			contextCount++
		}
	}
	if contextCount != 1 {
		t.Fatalf("durable context entries = %d, want 1", contextCount)
	}
	if len(entries) != 4 {
		t.Fatalf("durable entries = %d, want context, user, assistant, run", len(entries))
	}
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 2 {
		t.Fatalf("append batch sizes = %v, want [2 2]", batchSizes(batches))
	}
}

func llmUserText(t *testing.T, message llm.Message) string {
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
