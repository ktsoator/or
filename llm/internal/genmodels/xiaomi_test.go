package main

import (
	"reflect"
	"testing"
)

func TestNormalizeXiaomiToggleThinking(t *testing.T) {
	source := sourceModel{
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: []sourceReasoningOption{{Type: "toggle"}},
	}
	wantLevels := unsupportedThinkingLevels("minimal", "low", "medium")

	for provider := range xiaomiProviders {
		t.Run(provider, func(t *testing.T) {
			candidate := normalize("mimo-v2.5-pro", source, providerRule{
				Provider:  provider,
				Protocol:  "openai-completions",
				Normalize: normalizeXiaomiModel,
			})

			if !reflect.DeepEqual(candidate.ThinkingLevelMap, wantLevels) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, wantLevels)
			}
			if candidate.Compat.SupportsReasoningEffort == nil || *candidate.Compat.SupportsReasoningEffort {
				t.Fatalf("SupportsReasoningEffort = %v, want false", candidate.Compat.SupportsReasoningEffort)
			}
			if candidate.Compat.Kind != "openai" {
				t.Fatalf("compatibility kind = %q, want openai", candidate.Compat.Kind)
			}
			if candidate.Compat.ThinkingFormat != "xiaomi" {
				t.Fatalf("ThinkingFormat = %q, want xiaomi", candidate.Compat.ThinkingFormat)
			}
			if candidate.Compat.RequiresReasoningContentOnAssistantMessages == nil ||
				!*candidate.Compat.RequiresReasoningContentOnAssistantMessages {
				t.Fatal("Xiaomi toggle must replay assistant reasoning content")
			}
		})
	}
}

func TestNormalizeXiaomiToggleRequiresRouteMetadata(t *testing.T) {
	tests := []struct {
		name    string
		model   model
		options []sourceReasoningOption
	}{
		{
			name:  "missing toggle metadata",
			model: model{Provider: "xiaomi", Protocol: "openai-completions", Reasoning: true},
		},
		{
			name:    "different provider",
			model:   model{Provider: "opencode-go", Protocol: "openai-completions", Reasoning: true},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
		{
			name:    "different protocol",
			model:   model{Provider: "xiaomi", Protocol: "anthropic-messages", Reasoning: true},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
		{
			name:    "non-reasoning model",
			model:   model{Provider: "xiaomi", Protocol: "openai-completions"},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
		{
			name:  "toggle and effort metadata",
			model: model{Provider: "xiaomi", Protocol: "openai-completions", Reasoning: true},
			options: []sourceReasoningOption{
				{Type: "toggle"},
				{Type: "effort"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.model
			applyXiaomiRequestCompatibility(&test.model, test.options)
			if !reflect.DeepEqual(test.model, before) {
				t.Fatalf("route changed without verified Xiaomi toggle metadata:\n got: %#v\nwant: %#v", test.model, before)
			}
		})
	}
}
