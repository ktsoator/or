package main

import (
	"reflect"
	"testing"
)

func TestFromCopilotUsesFixedThinkingForChatCompletions(t *testing.T) {
	low, medium, high := "low", "medium", "high"
	grok := sourceModel{
		Name:             "Grok 4.5",
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: reasoningValues(&low, &medium, &high),
	}
	claude := sourceModel{Name: "Claude Haiku 4.5", ToolCall: true, Reasoning: true}
	catalog := map[string]sourceProvider{
		githubCopilotSource: {Models: map[string]sourceModel{
			"grok-4.5":         grok,
			"claude-haiku-4.5": claude,
		}},
	}

	models := fromCopilot(catalog)
	if len(models) != 2 {
		t.Fatalf("generated %d models, want 2", len(models))
	}
	byID := make(map[string]model, len(models))
	for _, candidate := range models {
		byID[candidate.ID] = candidate
	}

	gotGrok := byID["grok-4.5"]
	wantLevels := unsupportedThinkingLevels("off", "minimal", "low", "medium")
	if gotGrok.Protocol != "openai-completions" ||
		!reflect.DeepEqual(gotGrok.ThinkingLevelMap, wantLevels) {
		t.Fatalf("Grok route = %#v, want fixed Chat Completions thinking", gotGrok)
	}
	if gotGrok.Compat.SupportsReasoningEffort == nil || *gotGrok.Compat.SupportsReasoningEffort {
		t.Fatal("Copilot Chat Completions must not advertise reasoning_effort")
	}

	gotClaude := byID["claude-haiku-4.5"]
	if gotClaude.Protocol != "anthropic-messages" || gotClaude.ThinkingLevelMap != nil {
		t.Fatalf("Claude route = %#v, want unchanged Anthropic thinking", gotClaude)
	}
}
