package main

import (
	"reflect"
	"testing"
)

func TestApplyOpenCodeOverrides(t *testing.T) {
	tests := []struct {
		name                    string
		model                   model
		thinkingFormat          string
		requiresReasoning       bool
		supportsReasoningEffort bool
		unsupportedLevels       []string
		thinkingVisibility      string
	}{
		{
			name: "opencode go deepseek",
			model: model{
				ID: "deepseek-v4-flash", Provider: "opencode-go", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			thinkingFormat:          "deepseek",
			requiresReasoning:       true,
			supportsReasoningEffort: true,
		},
		{
			name: "opencode keeps native deepseek effort",
			model: model{
				ID: "deepseek-v4-pro", Provider: "opencode", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			requiresReasoning:       true,
			supportsReasoningEffort: true,
		},
		{
			name: "opencode kimi has toggle only",
			model: model{
				ID: "kimi-k2.6", Provider: "opencode", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			thinkingFormat:          "deepseek",
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"minimal", "low", "medium"},
		},
		{
			name: "opencode go kimi has toggle only",
			model: model{
				ID: "kimi-k2.6", Provider: "opencode-go", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			thinkingFormat:          "deepseek",
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"minimal", "low", "medium"},
		},
		{
			name: "opencode go minimax m2.7 has fixed hidden reasoning",
			model: model{
				ID: "minimax-m2.7", Provider: "opencode-go", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"off", "minimal", "low", "medium"},
			thinkingVisibility:      "hidden",
		},
		{
			name: "opencode go glm is always thinking",
			model: model{
				ID: "glm-5.2", Provider: "opencode-go", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			supportsReasoningEffort: true,
			unsupportedLevels:       []string{"off", "minimal", "low", "medium"},
		},
		{
			name: "opencode grok uses server default",
			model: model{
				ID: "grok-build-0.1", Provider: "opencode", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"off", "minimal", "low", "medium"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applyOpenCodeOverrides(&test.model)
			if test.model.Compat.ThinkingFormat != test.thinkingFormat {
				t.Fatalf("ThinkingFormat = %q, want %q", test.model.Compat.ThinkingFormat, test.thinkingFormat)
			}
			gotRequiresReasoning := test.model.Compat.RequiresReasoningContentOnAssistantMessages != nil &&
				*test.model.Compat.RequiresReasoningContentOnAssistantMessages
			if gotRequiresReasoning != test.requiresReasoning {
				t.Fatalf("RequiresReasoningContentOnAssistantMessages = %v, want %v", gotRequiresReasoning, test.requiresReasoning)
			}
			gotSupportsEffort := test.model.Compat.SupportsReasoningEffort == nil || *test.model.Compat.SupportsReasoningEffort
			if gotSupportsEffort != test.supportsReasoningEffort {
				t.Fatalf("SupportsReasoningEffort = %v, want %v", gotSupportsEffort, test.supportsReasoningEffort)
			}
			for _, level := range test.unsupportedLevels {
				value, present := test.model.ThinkingLevelMap[level]
				if !present || value != nil {
					t.Errorf("ThinkingLevelMap[%q] = %v, want explicit nil", level, value)
				}
			}
			if test.model.ThinkingVisibility != test.thinkingVisibility {
				t.Errorf("ThinkingVisibility = %q, want %q", test.model.ThinkingVisibility, test.thinkingVisibility)
			}
		})
	}
}

func TestApplyOpenCodeOverridesIgnoresOtherProtocols(t *testing.T) {
	model := model{
		ID: "kimi-k2.6", Provider: "opencode", Protocol: "anthropic-messages",
	}
	applyOpenCodeOverrides(&model)
	if model.Compat.ThinkingFormat != "" || model.Compat.SupportsReasoningEffort != nil ||
		model.ThinkingLevelMap != nil || model.ThinkingVisibility != "" {
		t.Fatalf("unexpected override: %#v", model)
	}
}

func TestFromOpenCodeAppliesHy3ReasoningOptions(t *testing.T) {
	none, low, high := "none", "low", "high"
	source := sourceModel{
		Name: "Hy3", ToolCall: true, Reasoning: true,
		ReasoningOptions: reasoningValues(&none, &low, &high),
	}
	catalog := map[string]sourceProvider{
		"opencode-go": {Models: map[string]sourceModel{"hy3": source}},
	}

	models := fromOpenCode(catalog)
	if len(models) != 1 {
		t.Fatalf("generated %d models, want 1", len(models))
	}
	want := map[string]*string{
		"off": stringPointer("none"), "minimal": nil, "low": stringPointer("low"),
		"medium": nil, "high": stringPointer("high"), "xhigh": nil, "max": nil,
	}
	if !reflect.DeepEqual(models[0].ThinkingLevelMap, want) {
		t.Fatalf("ThinkingLevelMap = %#v, want %#v", models[0].ThinkingLevelMap, want)
	}
}

func TestFromOpenCodeManualReasoningOverrideWins(t *testing.T) {
	none, low := "none", "low"
	source := sourceModel{
		ToolCall: true, Reasoning: true,
		ReasoningOptions: reasoningValues(&none, &low),
	}
	catalog := map[string]sourceProvider{
		"opencode-go": {Models: map[string]sourceModel{"glm-5.2": source}},
	}

	models := fromOpenCode(catalog)
	if len(models) != 1 {
		t.Fatalf("generated %d models, want 1", len(models))
	}
	want := map[string]*string{
		"off": nil, "minimal": nil, "low": nil, "medium": nil,
		"high": stringPointer("high"), "max": stringPointer("max"),
	}
	if !reflect.DeepEqual(models[0].ThinkingLevelMap, want) {
		t.Fatalf("ThinkingLevelMap = %#v, want manual override %#v", models[0].ThinkingLevelMap, want)
	}
}

func TestApplyOpenCodeOverridesCorrectsClaudeContext(t *testing.T) {
	for _, provider := range []string{"opencode", "opencode-go"} {
		for _, modelID := range []string{"claude-sonnet-4", "claude-sonnet-4-5"} {
			t.Run(provider+"/"+modelID, func(t *testing.T) {
				candidate := model{
					ID: modelID, Provider: provider, Protocol: "anthropic-messages", ContextWindow: 1_000_000,
				}
				applyOpenCodeOverrides(&candidate)
				if candidate.ContextWindow != 200_000 {
					t.Fatalf("ContextWindow = %d, want 200000", candidate.ContextWindow)
				}
			})
		}
	}

	control := model{
		ID: "claude-sonnet-4-6", Provider: "opencode", Protocol: "anthropic-messages", ContextWindow: 1_000_000,
	}
	applyOpenCodeOverrides(&control)
	if control.ContextWindow != 1_000_000 {
		t.Fatalf("claude-sonnet-4-6 ContextWindow = %d, want unchanged", control.ContextWindow)
	}
}
