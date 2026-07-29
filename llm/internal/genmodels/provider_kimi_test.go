package main

import (
	"reflect"
	"testing"
)

func TestApplyOverridesForKimiCodingCompatibility(t *testing.T) {
	tests := []struct {
		name                    string
		model                   model
		wantAdaptive            bool
		wantAllowEmptySignature bool
	}{
		{
			name:                    "k3",
			model:                   model{ID: "k3", Provider: "kimi-coding", Protocol: "anthropic-messages"},
			wantAdaptive:            true,
			wantAllowEmptySignature: true,
		},
		{
			name: "k3-256k",
			model: model{
				ID:       "k3-256k",
				Provider: "kimi-coding",
				Protocol: "anthropic-messages",
				Compat: compatibility{
					Kind:                "anthropic",
					AllowEmptySignature: boolp(true),
				},
			},
			wantAdaptive: true,
		},
		{
			name:                    "kimi-for-coding",
			model:                   model{ID: "kimi-for-coding", Provider: "kimi-coding", Protocol: "anthropic-messages"},
			wantAdaptive:            true,
			wantAllowEmptySignature: true,
		},
		{
			name:         "kimi-for-coding-highspeed",
			model:        model{ID: "kimi-for-coding-highspeed", Provider: "kimi-coding", Protocol: "anthropic-messages"},
			wantAdaptive: true,
		},
		{
			name:  "other provider",
			model: model{ID: "k3", Provider: "moonshotai", Protocol: "anthropic-messages"},
		},
		{
			name:  "other protocol",
			model: model{ID: "k3", Provider: "kimi-coding", Protocol: "openai-completions"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			models := []model{test.model}
			applyOverrides(models)
			compatibility := models[0].Compat

			gotAdaptive := compatibility.ForceAdaptiveThinking != nil &&
				*compatibility.ForceAdaptiveThinking
			if gotAdaptive != test.wantAdaptive {
				t.Errorf("ForceAdaptiveThinking = %v, want %v", gotAdaptive, test.wantAdaptive)
			}

			gotAllowEmptySignature := compatibility.AllowEmptySignature != nil &&
				*compatibility.AllowEmptySignature
			if gotAllowEmptySignature != test.wantAllowEmptySignature {
				t.Errorf(
					"AllowEmptySignature = %v, want %v",
					gotAllowEmptySignature,
					test.wantAllowEmptySignature,
				)
			}

			wantKind := ""
			if test.wantAdaptive {
				wantKind = "anthropic"
			}
			if compatibility.Kind != wantKind {
				t.Errorf("compatibility kind = %q, want %q", compatibility.Kind, wantKind)
			}
		})
	}
}

func TestNormalizeMoonshotToggleThinking(t *testing.T) {
	source := sourceModel{
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: []sourceReasoningOption{{Type: "toggle"}},
	}
	wantLevels := unsupportedThinkingLevels("minimal", "low", "medium")

	for provider := range moonshotProviders {
		for _, modelID := range []string{"kimi-k2.5", "kimi-k2.6"} {
			t.Run(provider+"/"+modelID, func(t *testing.T) {
				candidate := normalize(modelID, source, providerRule{
					Provider: provider,
					Protocol: "openai-completions",
					Compat:   moonshotCompat(),
				})

				if !reflect.DeepEqual(candidate.ThinkingLevelMap, wantLevels) {
					t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, wantLevels)
				}
				if candidate.Compat.SupportsReasoningEffort == nil || *candidate.Compat.SupportsReasoningEffort {
					t.Fatalf("SupportsReasoningEffort = %v, want false", candidate.Compat.SupportsReasoningEffort)
				}
				if candidate.Compat.ThinkingFormat != "deepseek" {
					t.Fatalf("ThinkingFormat = %q, want deepseek", candidate.Compat.ThinkingFormat)
				}
				if candidate.Compat.RequiresReasoningContentOnAssistantMessages != nil {
					t.Fatalf(
						"RequiresReasoningContentOnAssistantMessages = %v, want unset",
						candidate.Compat.RequiresReasoningContentOnAssistantMessages,
					)
				}
			})
		}
	}
}

func TestNormalizeKimiEffortMetadata(t *testing.T) {
	low, high, max := "low", "high", "max"
	source := sourceModel{
		ToolCall:  true,
		Reasoning: true,
		ReasoningOptions: []sourceReasoningOption{
			{Type: "toggle"},
			{Type: "effort", Values: []*string{&low, &high, &max}},
		},
	}
	wantLevels := effortThinkingLevelMap(source.ReasoningOptions)

	tests := []struct {
		name     string
		id       string
		rule     providerRule
		adaptive bool
	}{
		{
			name: "Kimi Coding K3",
			id:   "k3",
			rule: providerRule{
				Provider: "kimi-coding", Protocol: "anthropic-messages",
			},
			adaptive: true,
		},
		{
			name: "Moonshot Kimi K3",
			id:   "kimi-k3",
			rule: providerRule{
				Provider: "moonshotai", Protocol: "openai-completions", Compat: moonshotCompat(),
			},
		},
		{
			name: "Moonshot CN Kimi K3",
			id:   "kimi-k3",
			rule: providerRule{
				Provider: "moonshotai-cn", Protocol: "openai-completions", Compat: moonshotCompat(),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := normalize(test.id, source, test.rule)
			models := []model{candidate}
			applyOverrides(models)
			candidate = models[0]

			if !reflect.DeepEqual(candidate.ThinkingLevelMap, wantLevels) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, wantLevels)
			}
			if test.adaptive {
				if candidate.Compat.ForceAdaptiveThinking == nil || !*candidate.Compat.ForceAdaptiveThinking {
					t.Fatal("Kimi Coding model is not marked adaptive")
				}
				return
			}
			if candidate.Compat.SupportsReasoningEffort == nil || !*candidate.Compat.SupportsReasoningEffort {
				t.Fatal("Moonshot Kimi K3 does not support reasoning_effort")
			}
			if candidate.Compat.ThinkingFormat != "" {
				t.Fatalf("ThinkingFormat = %q, want standard OpenAI", candidate.Compat.ThinkingFormat)
			}
			if candidate.Compat.RequiresReasoningContentOnAssistantMessages == nil ||
				!*candidate.Compat.RequiresReasoningContentOnAssistantMessages {
				t.Fatal("Moonshot Kimi K3 must replay assistant reasoning content")
			}
		})
	}
}

func TestKimiK27CodeDisablesOff(t *testing.T) {
	for _, provider := range []string{"moonshotai", "moonshotai-cn"} {
		for _, id := range []string{"kimi-k2.7-code", "kimi-k2.7-code-highspeed"} {
			t.Run(provider+"/"+id, func(t *testing.T) {
				candidate := model{ID: id, Provider: provider, Protocol: "openai-completions"}
				applyKimiOverrides(&candidate)
				value, ok := candidate.ThinkingLevelMap["off"]
				if !ok || value != nil {
					t.Fatalf("ThinkingLevelMap[off] = %v (present %v), want explicit nil", value, ok)
				}
			})
		}
	}
}
