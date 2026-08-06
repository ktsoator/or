package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestModelActivatedSkillSurvivesCompactionAndRestore(t *testing.T) {
	ctx := context.Background()
	store := &transcript.Memory{}
	compactor := &recordingCompactor{}
	const protectedBody = "ORIGINAL PROTECTED BODY: keep this exact instruction"
	skill := skills.Skill{
		Name: "review", Description: "Review code", Content: protectedBody,
		Dir: "/skills/review", Path: "/skills/review/SKILL.md",
	}
	var inputs []llm.Context
	modelCalls := 0
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model", ContextWindow: 2000},
		Tools: []tools.Tool{}, Store: store, Compactor: compactor,
		Skills: []skills.Skill{skill},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			inputs = append(inputs, input)
			modelCalls++
			if modelCalls == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{&llm.ToolCall{
					ID: "skill-call", Name: skills.ToolName,
					Arguments: map[string]any{"name": "review"},
				}}
				return finalEvents(llm.EventDone, &message), nil
			}
			return assistantEvents(model, "answer "+strings.Repeat("a", 1200)), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "activate review"); err != nil {
		t.Fatal(err)
	}
	activatedEntries := 0
	for _, entry := range session.Entries() {
		if entry.Type == transcript.ContextEntry && entry.Context != nil &&
			entry.Context.Kind == string(modelcontext.ActivatedSkill) {
			activatedEntries++
			if entry.Context.Path != "review" || !strings.Contains(entry.Context.Rendered, protectedBody) {
				t.Fatalf("activated Skill entry = %#v", entry.Context)
			}
		}
	}
	if activatedEntries != 1 {
		t.Fatalf("activated Skill entries = %d, want 1", activatedEntries)
	}

	if err := session.Prompt(ctx, "new request "+strings.Repeat("u", 1200)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Compact(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) != 1 || messagesContain(compactor.requests[0].Messages, protectedBody) {
		t.Fatal("compaction prompt retained the full Skill instructions")
	}
	if err := session.Prompt(ctx, "after compaction"); err != nil {
		t.Fatal(err)
	}
	if !contextContains(inputs[len(inputs)-1], protectedBody) {
		t.Fatal("activated Skill disappeared after compaction")
	}

	var restoredInput llm.Context
	restoredSkill := skill
	restoredSkill.Content = "UPDATED BODY THAT MUST NOT REPLACE THE ACTIVE SNAPSHOT"
	restored, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model", ContextWindow: 2000},
		Tools: []tools.Tool{}, Store: store, Compactor: compactor,
		Skills: []skills.Skill{restoredSkill},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			restoredInput = input
			return assistantEvents(model, "restored"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Prompt(ctx, "after restore"); err != nil {
		t.Fatal(err)
	}
	if !contextContains(restoredInput, protectedBody) ||
		contextContains(restoredInput, restoredSkill.Content) {
		t.Fatal("restore did not keep the original activated Skill snapshot")
	}
}

func contextContains(input llm.Context, text string) bool {
	for _, message := range input.Messages {
		user, ok := message.(*llm.UserMessage)
		if !ok {
			continue
		}
		for _, content := range user.Content {
			if block, ok := content.(*llm.TextContent); ok && strings.Contains(block.Text, text) {
				return true
			}
		}
	}
	return false
}

func messagesContain(messages []agent.AgentMessage, text string) bool {
	for _, message := range messages {
		value, ok := agent.ToLLM(message)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case *llm.UserMessage:
			for _, content := range typed.Content {
				if block, ok := content.(*llm.TextContent); ok && strings.Contains(block.Text, text) {
					return true
				}
			}
		case *llm.ToolResultMessage:
			for _, content := range typed.Content {
				if block, ok := content.(*llm.TextContent); ok && strings.Contains(block.Text, text) {
					return true
				}
			}
		}
	}
	return false
}
