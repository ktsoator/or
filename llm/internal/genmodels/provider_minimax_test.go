package main

import (
	"reflect"
	"testing"
)

func TestNormalizeMiniMaxToggleThinking(t *testing.T) {
	source := sourceModel{
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: []sourceReasoningOption{{Type: "toggle"}},
	}
	wantLevels := unsupportedThinkingLevels("minimal", "low", "medium")
	tests := []struct {
		provider string
		modelID  string
	}{
		{provider: "minimax", modelID: "MiniMax-M3"},
		{provider: "minimax-cn", modelID: "MiniMax-M3"},
		{provider: "opencode-go", modelID: "minimax-m3"},
	}

	for _, test := range tests {
		t.Run(test.provider+"/"+test.modelID, func(t *testing.T) {
			models := []model{normalize(test.modelID, source, providerRule{
				Provider: test.provider,
				Protocol: "anthropic-messages",
			})}
			applyOverrides(models)
			candidate := models[0]

			if !reflect.DeepEqual(candidate.ThinkingLevelMap, wantLevels) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, wantLevels)
			}
			if candidate.Compat.Kind != "" || candidate.Compat.ForceAdaptiveThinking != nil {
				t.Fatalf("unexpected MiniMax compatibility override: %#v", candidate.Compat)
			}
		})
	}
}

func TestMiniMaxToggleThinkingRequiresVerifiedRoute(t *testing.T) {
	tests := []struct {
		name    string
		model   model
		options []sourceReasoningOption
	}{
		{
			name:  "missing toggle metadata",
			model: model{ID: "MiniMax-M3", Provider: "minimax", Protocol: "anthropic-messages", Reasoning: true},
		},
		{
			name:  "toggle and budget metadata",
			model: model{ID: "MiniMax-M3", Provider: "minimax", Protocol: "anthropic-messages", Reasoning: true},
			options: []sourceReasoningOption{
				{Type: "toggle"},
				{Type: "budget_tokens"},
			},
		},
		{
			name:    "OpenCode Go other model family",
			model:   model{ID: "qwen3.7-plus", Provider: "opencode-go", Protocol: "anthropic-messages", Reasoning: true},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
		{
			name:    "different gateway route",
			model:   model{ID: "minimax-m3", Provider: "opencode", Protocol: "anthropic-messages", Reasoning: true},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
		{
			name:    "different protocol",
			model:   model{ID: "MiniMax-M3", Provider: "minimax", Protocol: "openai-completions", Reasoning: true},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
		{
			name:    "non-reasoning model",
			model:   model{ID: "MiniMax-M3", Provider: "minimax", Protocol: "anthropic-messages"},
			options: []sourceReasoningOption{{Type: "toggle"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applyMiniMaxThinkingMetadata(&test.model, test.options)
			if test.model.ThinkingLevelMap != nil {
				t.Fatalf("unexpected MiniMax toggle metadata: %#v", test.model.ThinkingLevelMap)
			}
		})
	}
}

func TestApplyMiniMaxOverridesDisablesOffForM27(t *testing.T) {
	for _, provider := range []string{"minimax", "minimax-cn"} {
		for _, id := range []string{"MiniMax-M2.7", "MiniMax-M2.7-highspeed"} {
			t.Run(provider+"/"+id, func(t *testing.T) {
				high := "high"
				candidate := model{
					ID:       id,
					Provider: provider,
					Protocol: "anthropic-messages",
					ThinkingLevelMap: map[string]*string{
						"high": &high,
					},
				}

				applyMiniMaxOverrides(&candidate)

				value, present := candidate.ThinkingLevelMap["off"]
				if !present || value != nil {
					t.Fatalf("ThinkingLevelMap[off] = %v (present %v), want explicit nil", value, present)
				}
				if value := candidate.ThinkingLevelMap["high"]; value == nil || *value != "high" {
					t.Fatalf("existing high mapping = %v, want preserved", value)
				}
			})
		}
	}
}

func TestApplyMiniMaxOverridesLeavesM3ToggleEnabled(t *testing.T) {
	candidate := model{ID: "MiniMax-M3", Provider: "minimax-cn", Protocol: "anthropic-messages"}
	applyMiniMaxOverrides(&candidate)
	if _, present := candidate.ThinkingLevelMap["off"]; present {
		t.Fatalf("ThinkingLevelMap[off] is present for M3: %#v", candidate.ThinkingLevelMap)
	}
}

func TestApplyMiniMaxOverridesIgnoresOtherRoutes(t *testing.T) {
	tests := []model{
		{ID: "MiniMax-M2.7", Provider: "together", Protocol: "openai-completions"},
		{ID: "MiniMax-M2.7", Provider: "minimax-cn", Protocol: "openai-completions"},
	}
	for _, candidate := range tests {
		applyMiniMaxOverrides(&candidate)
		if candidate.ThinkingLevelMap != nil {
			t.Fatalf("unexpected override for %s/%s (%s): %#v", candidate.Provider, candidate.ID, candidate.Protocol, candidate.ThinkingLevelMap)
		}
	}
}
