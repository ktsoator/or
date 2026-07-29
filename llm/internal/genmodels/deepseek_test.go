package main

import (
	"reflect"
	"testing"
)

func TestNormalizeDeepSeekThinkingMetadata(t *testing.T) {
	high, max, none := "high", "max", "none"
	tests := []struct {
		name           string
		options        []sourceReasoningOption
		wantLevels     map[string]*string
		supportsEffort *bool
	}{
		{
			name:           "toggle",
			options:        []sourceReasoningOption{{Type: "toggle"}},
			wantLevels:     unsupportedThinkingLevels("minimal", "low", "medium"),
			supportsEffort: boolp(false),
		},
		{
			name: "toggle and effort",
			options: []sourceReasoningOption{
				{Type: "toggle"},
				{Type: "effort", Values: []*string{&high, &max}},
			},
			wantLevels:     explicitThinkingLevels([]string{"off", "high", "max"}),
			supportsEffort: boolp(true),
		},
		{
			name: "effort with explicit none",
			options: []sourceReasoningOption{
				{Type: "effort", Values: []*string{&none, &high}},
			},
			wantLevels:     explicitThinkingLevels([]string{"off", "high"}),
			supportsEffort: boolp(true),
		},
		{
			name: "provider default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := sourceModel{
				ToolCall:         true,
				Reasoning:        true,
				ReasoningOptions: test.options,
			}
			candidate := normalize("deepseek-model", source, providerRule{
				Provider:  "deepseek",
				Protocol:  "openai-completions",
				BaseURL:   "https://api.deepseek.com",
				Normalize: normalizeDeepSeekModel,
			})

			if !reflect.DeepEqual(candidate.ThinkingLevelMap, test.wantLevels) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, test.wantLevels)
			}
			if !sameBoolPointer(candidate.Compat.SupportsReasoningEffort, test.supportsEffort) {
				t.Fatalf(
					"SupportsReasoningEffort = %v, want %v",
					candidate.Compat.SupportsReasoningEffort,
					test.supportsEffort,
				)
			}
			if candidate.Compat.Kind != "openai" {
				t.Fatalf("compatibility kind = %q, want openai", candidate.Compat.Kind)
			}
			if candidate.Compat.ThinkingFormat != "deepseek" {
				t.Fatalf("ThinkingFormat = %q, want deepseek", candidate.Compat.ThinkingFormat)
			}
			if candidate.Compat.RequiresReasoningContentOnAssistantMessages == nil ||
				!*candidate.Compat.RequiresReasoningContentOnAssistantMessages {
				t.Fatal("DeepSeek reasoning routes must replay assistant reasoning_content")
			}
		})
	}
}

func TestNormalizeDeepSeekIgnoresOtherRoutes(t *testing.T) {
	high := "high"
	tests := []model{
		{ID: "deepseek-v4-pro", Provider: "opencode-go", Protocol: "openai-completions", Reasoning: true},
		{ID: "deepseek-v4-pro", Provider: "deepseek", Protocol: "anthropic-messages", Reasoning: true},
		{ID: "deepseek-chat", Provider: "deepseek", Protocol: "openai-completions"},
	}
	options := []sourceReasoningOption{
		{Type: "toggle"},
		{Type: "effort", Values: []*string{&high}},
	}

	for _, candidate := range tests {
		before := candidate
		applyDeepSeekRequestCompatibility(&candidate, options)
		if !reflect.DeepEqual(candidate, before) {
			t.Fatalf(
				"route %s/%s (%s) changed:\n got: %#v\nwant: %#v",
				candidate.Provider,
				candidate.ID,
				candidate.Protocol,
				candidate,
				before,
			)
		}
	}
}
