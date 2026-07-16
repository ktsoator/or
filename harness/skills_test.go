package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/harness"
	"github.com/ktsoator/or/llm"
)

func TestSkillInvocationInjectsContent(t *testing.T) {
	ctx := context.Background()
	rec := &recordingStream{turns: [][]llm.Event{textTurn("ok")}}
	h, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: rec.fn(),
		Skills: []harness.Skill{
			{Name: "review", Description: "review code", Content: "Review the diff carefully."},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := h.Skill(ctx, "review", "Focus on error handling."); err != nil {
		t.Fatalf("Skill() error = %v", err)
	}

	// The injected user turn carries the skill content and the extra instruction.
	prompt := messageText(t, h.Snapshot().Messages[0])
	if !strings.Contains(prompt, "Review the diff carefully.") {
		t.Fatalf("invoked turn missing skill content: %q", prompt)
	}
	if !strings.Contains(prompt, "Focus on error handling.") {
		t.Fatalf("invoked turn missing additional instructions: %q", prompt)
	}
}

func TestSkillUnknownErrors(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{Model: testModel, StreamFn: scriptedStream("x")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.Skill(ctx, "ghost"); err == nil {
		t.Fatal("Skill(\"ghost\") = nil, want error for unknown skill")
	}
}

func TestPromptFromTemplateSubstitutesArgs(t *testing.T) {
	ctx := context.Background()
	rec := &recordingStream{turns: [][]llm.Event{textTurn("ok")}}
	h, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: rec.fn(),
		PromptTemplates: []harness.PromptTemplate{
			{Name: "greet", Content: "Say hi to $1 about $ARGUMENTS."},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := h.PromptFromTemplate(ctx, "greet", "Alice", "Bob"); err != nil {
		t.Fatalf("PromptFromTemplate() error = %v", err)
	}

	prompt := messageText(t, h.Snapshot().Messages[0])
	if prompt != "Say hi to Alice about Alice Bob." {
		t.Fatalf("substituted prompt = %q", prompt)
	}
}

func TestFormatSkillsForSystemPrompt(t *testing.T) {
	skills := []harness.Skill{
		{Name: "review", Description: "review code"},
		{Name: "internal", Description: "hidden", DisableModelInvocation: true},
	}
	out := harness.FormatSkillsForSystemPrompt(skills)
	if !strings.Contains(out, "<name>review</name>") {
		t.Fatalf("listing missing visible skill: %q", out)
	}
	if strings.Contains(out, "internal") {
		t.Fatalf("listing should omit model-disabled skill: %q", out)
	}

	// No visible skills yields an empty string.
	if got := harness.FormatSkillsForSystemPrompt(skills[1:]); got != "" {
		t.Fatalf("expected empty listing, got %q", got)
	}
}

func TestSkillsListedInDynamicPrompt(t *testing.T) {
	ctx := context.Background()
	rec := &recordingStream{turns: [][]llm.Event{textTurn("ok")}}
	h, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: rec.fn(),
		Skills:   []harness.Skill{{Name: "review", Description: "review code"}},
		BuildSystemPrompt: func(info harness.TurnInfo) string {
			return "Base.\n\n" + harness.FormatSkillsForSystemPrompt(info.Skills)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if len(rec.prompts) != 1 || !strings.Contains(rec.prompts[0], "<name>review</name>") {
		t.Fatalf("model system prompt missing skill listing: %#v", rec.prompts)
	}
}
