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
			name: "opencode go qwen has toggle only",
			model: model{
				ID: "qwen3.6-plus", Provider: "opencode-go", Protocol: "openai-completions", Reasoning: true,
				Compat: compatibility{Kind: "openai", ThinkingFormat: "qwen"},
			},
			thinkingFormat:          "qwen",
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"minimal", "low", "medium"},
		},
		{
			name: "opencode go mimo has toggle only",
			model: model{
				ID: "mimo-v2.5", Provider: "opencode-go", Protocol: "openai-completions", Reasoning: true,
				Compat: compatibility{Kind: "openai"},
			},
			thinkingFormat:          "deepseek",
			requiresReasoning:       true,
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"minimal", "low", "medium"},
		},
		{
			name: "opencode go mimo pro has fixed thinking",
			model: model{
				ID: "mimo-v2.5-pro", Provider: "opencode-go", Protocol: "openai-completions", Reasoning: true,
				Compat: compatibility{Kind: "openai"},
			},
			requiresReasoning:       true,
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"off", "minimal", "low", "medium"},
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
				ID: "grok-build-0.1", Provider: "opencode", Protocol: "openai-completions", Reasoning: true,
				Compat: compatibility{Kind: "openai"},
			},
			supportsReasoningEffort: false,
			unsupportedLevels:       []string{"off", "minimal", "low", "medium"},
		},
		{
			name: "unverified opencode controls use fixed provider default",
			model: model{
				ID: "glm-5", Provider: "opencode", Protocol: "openai-completions", Reasoning: true,
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
	applyOverrides(models)
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
	applyOverrides(models)
	if len(models) != 1 {
		t.Fatalf("generated %d models, want 1", len(models))
	}
	want := map[string]*string{
		"off": nil, "minimal": nil, "low": nil, "medium": nil,
		"high": stringPointer("high"), "xhigh": nil, "max": stringPointer("max"),
	}
	if !reflect.DeepEqual(models[0].ThinkingLevelMap, want) {
		t.Fatalf("ThinkingLevelMap = %#v, want manual override %#v", models[0].ThinkingLevelMap, want)
	}
}

func TestFromOpenCodeKeepsMiniMaxRoutesDistinct(t *testing.T) {
	m27 := sourceModel{Name: "MiniMax M2.7", ToolCall: true, Reasoning: true}
	m27.Provider.NPM = "@ai-sdk/anthropic"
	m3 := sourceModel{
		Name:             "MiniMax M3",
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: []sourceReasoningOption{{Type: "toggle"}},
	}
	m3.Provider.NPM = "@ai-sdk/anthropic"
	catalog := map[string]sourceProvider{
		"opencode-go": {Models: map[string]sourceModel{
			"minimax-m2.7": m27,
			"minimax-m3":   m3,
		}},
	}

	models := fromOpenCode(catalog)
	applyOverrides(models)
	if len(models) != 2 {
		t.Fatalf("generated %d models, want 2", len(models))
	}
	byID := make(map[string]model, len(models))
	for _, candidate := range models {
		byID[candidate.ID] = candidate
	}

	gotM27 := byID["minimax-m2.7"]
	if gotM27.Protocol != "openai-completions" || gotM27.BaseURL != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("M2.7 route = %s %s, want OpenAI /v1", gotM27.Protocol, gotM27.BaseURL)
	}
	if gotM27.ThinkingVisibility != "hidden" ||
		!reflect.DeepEqual(gotM27.ThinkingLevelMap, unsupportedThinkingLevels("off", "minimal", "low", "medium")) {
		t.Fatalf("M2.7 thinking profile = %#v visibility=%q, want fixed hidden", gotM27.ThinkingLevelMap, gotM27.ThinkingVisibility)
	}

	gotM3 := byID["minimax-m3"]
	if gotM3.Protocol != "anthropic-messages" || gotM3.BaseURL != "https://opencode.ai/zen/go" {
		t.Fatalf("M3 route = %s %s, want Anthropic base route", gotM3.Protocol, gotM3.BaseURL)
	}
	if !reflect.DeepEqual(gotM3.ThinkingLevelMap, unsupportedThinkingLevels("minimal", "low", "medium")) {
		t.Fatalf("M3 thinking profile = %#v, want binary toggle", gotM3.ThinkingLevelMap)
	}
}

func TestFromOpenCodeRoutesOpenAIModelsThroughResponses(t *testing.T) {
	low, medium, high := "low", "medium", "high"
	grok := sourceModel{
		Name:             "Grok 4.5",
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: reasoningValues(&low, &medium, &high),
	}
	grok.Provider.NPM = "@ai-sdk/openai"
	google := sourceModel{Name: "Gemini", ToolCall: true}
	google.Provider.NPM = "@ai-sdk/google"
	catalog := map[string]sourceProvider{
		"opencode": {Models: map[string]sourceModel{
			"grok-4.5": grok,
			"gemini":   google,
		}},
	}

	models := fromOpenCode(catalog)
	if len(models) != 1 {
		t.Fatalf("generated %d models, want only the OpenAI Responses model", len(models))
	}
	got := models[0]
	if got.ID != "grok-4.5" || got.Provider != "opencode" ||
		got.Protocol != "openai-responses" || got.BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("Grok route = %#v, want opencode OpenAI Responses /v1", got)
	}
	wantLevels := explicitThinkingLevels([]string{"low", "medium", "high"})
	if !reflect.DeepEqual(got.ThinkingLevelMap, wantLevels) {
		t.Fatalf("Grok thinking levels = %#v, want %#v", got.ThinkingLevelMap, wantLevels)
	}
	if got.Compat != (compatibility{}) {
		t.Fatalf("Grok compatibility = %#v, want none for Responses", got.Compat)
	}
}

func TestFromOpenCodeResponsesWithoutControlsUsesProviderDefault(t *testing.T) {
	grok := sourceModel{Name: "Grok Build 0.1", ToolCall: true, Reasoning: true}
	grok.Provider.NPM = "@ai-sdk/openai"
	catalog := map[string]sourceProvider{
		"opencode": {Models: map[string]sourceModel{"grok-build-0.1": grok}},
	}

	models := fromOpenCode(catalog)
	applyOverrides(models)
	if len(models) != 1 {
		t.Fatalf("generated %d models, want 1", len(models))
	}
	want := unsupportedThinkingLevels("off", "minimal", "low", "medium")
	if !reflect.DeepEqual(models[0].ThinkingLevelMap, want) {
		t.Fatalf("ThinkingLevelMap = %#v, want fixed provider default %#v", models[0].ThinkingLevelMap, want)
	}
}

func TestApplyOpenCodeOverridesKeepsRoutesIndependent(t *testing.T) {
	candidate := model{
		ID: "mimo-v2.5", Provider: "xiaomi", Protocol: "openai-completions", Reasoning: true,
		Compat: compatibility{Kind: "openai", ThinkingFormat: "deepseek"},
	}
	before := candidate
	applyOpenCodeOverrides(&candidate)
	if !reflect.DeepEqual(candidate, before) {
		t.Fatalf("OpenCode override leaked into Xiaomi route:\n got: %#v\nwant: %#v", candidate, before)
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
