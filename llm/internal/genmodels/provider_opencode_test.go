package main

import "testing"

func TestApplyOpenCodeOverrides(t *testing.T) {
	tests := []struct {
		name                    string
		model                   model
		thinkingFormat          string
		requiresReasoning       bool
		supportsReasoningEffort bool
		unsupportedLevels       []string
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
			name: "opencode kimi",
			model: model{
				ID: "kimi-k2.6", Provider: "opencode", Protocol: "openai-completions",
				Compat: compatibility{Kind: "openai"},
			},
			thinkingFormat:          "deepseek",
			supportsReasoningEffort: false,
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
		})
	}
}

func TestApplyOpenCodeOverridesIgnoresOtherProtocols(t *testing.T) {
	model := model{
		ID: "kimi-k2.6", Provider: "opencode", Protocol: "anthropic-messages",
	}
	applyOpenCodeOverrides(&model)
	if model.Compat.ThinkingFormat != "" || model.Compat.SupportsReasoningEffort != nil || model.ThinkingLevelMap != nil {
		t.Fatalf("unexpected override: %#v", model)
	}
}
