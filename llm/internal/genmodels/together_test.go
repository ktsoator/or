package main

import (
	"reflect"
	"testing"
)

func TestApplyTogetherRequestCompatibility(t *testing.T) {
	tests := []struct {
		name             string
		id               string
		reasoning        bool
		thinkingFormat   string
		supportsEffort   bool
		unsupportedLevel []string
	}{
		{name: "non reasoning", id: "Qwen/Qwen2.5-7B-Instruct-Turbo"},
		{
			name: "fixed reasoning", id: "MiniMaxAI/MiniMax-M2.7", reasoning: true,
			thinkingFormat: "openai", unsupportedLevel: []string{"off", "minimal", "low", "medium"},
		},
		{
			name: "reasoning effort", id: "openai/gpt-oss-120b", reasoning: true,
			thinkingFormat: "openai", supportsEffort: true, unsupportedLevel: []string{"off", "minimal"},
		},
		{
			name: "toggle and effort", id: "deepseek-ai/DeepSeek-V4-Pro", reasoning: true,
			thinkingFormat: "together", supportsEffort: true,
			unsupportedLevel: []string{"minimal", "low", "medium", "xhigh"},
		},
		{
			name: "toggle only", id: "moonshotai/Kimi-K2.6", reasoning: true,
			thinkingFormat: "together", unsupportedLevel: []string{"minimal", "low", "medium"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := model{
				ID: test.id, Provider: "together", Protocol: "openai-completions", Reasoning: test.reasoning,
			}
			applyTogetherRequestCompatibility(&candidate)

			if candidate.Compat.ThinkingFormat != test.thinkingFormat {
				t.Fatalf("ThinkingFormat = %q, want %q", candidate.Compat.ThinkingFormat, test.thinkingFormat)
			}
			gotSupportsEffort := candidate.Compat.SupportsReasoningEffort != nil && *candidate.Compat.SupportsReasoningEffort
			if gotSupportsEffort != test.supportsEffort {
				t.Fatalf("SupportsReasoningEffort = %v, want %v", gotSupportsEffort, test.supportsEffort)
			}
			if candidate.Compat.SupportsStore == nil || *candidate.Compat.SupportsStore {
				t.Fatal("Together must disable store")
			}
			if candidate.Compat.SupportsDeveloperRole == nil || *candidate.Compat.SupportsDeveloperRole {
				t.Fatal("Together must disable developer role")
			}
			if candidate.Compat.SupportsStrictMode == nil || *candidate.Compat.SupportsStrictMode {
				t.Fatal("Together must disable strict mode")
			}
			if candidate.Compat.MaxTokensField != "max_tokens" {
				t.Fatalf("MaxTokensField = %q, want max_tokens", candidate.Compat.MaxTokensField)
			}
			for _, level := range test.unsupportedLevel {
				value, present := candidate.ThinkingLevelMap[level]
				if !present || value != nil {
					t.Errorf("ThinkingLevelMap[%q] = %v, want explicit nil", level, value)
				}
			}
		})
	}
}

func TestTogetherEffortMetadataUsesModelOptions(t *testing.T) {
	low, high := "low", "high"
	candidate := normalize("openai/gpt-oss-120b", sourceModel{
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: reasoningValues(&low, &high),
	}, providerRule{
		Provider: "together", Protocol: "openai-completions", BaseURL: "https://api.together.ai/v1",
		Normalize: normalizeTogetherModel,
	})
	want := map[string]*string{
		"off": nil, "minimal": nil, "low": stringPointer("low"), "medium": nil,
		"high": stringPointer("high"), "xhigh": nil, "max": nil,
	}
	if !reflect.DeepEqual(candidate.ThinkingLevelMap, want) {
		t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, want)
	}
}

func TestApplyTogetherRequestCompatibilityIgnoresOtherProviders(t *testing.T) {
	candidate := model{ID: "openai/gpt-oss-120b", Provider: "groq", Protocol: "openai-completions", Reasoning: true}
	applyTogetherRequestCompatibility(&candidate)
	if candidate.Compat.Kind != "" || candidate.ThinkingLevelMap != nil {
		t.Fatalf("unexpected override: %#v", candidate)
	}
}
