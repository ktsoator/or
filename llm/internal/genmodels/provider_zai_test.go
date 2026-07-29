package main

import (
	"reflect"
	"testing"
)

func TestNormalizeZAIToggleThinking(t *testing.T) {
	source := sourceModel{
		ToolCall:         true,
		Reasoning:        true,
		ReasoningOptions: []sourceReasoningOption{{Type: "toggle"}},
	}
	wantLevels := unsupportedThinkingLevels("minimal", "low", "medium")
	modelIDs := []string{"glm-4.5-air", "glm-4.7", "glm-5-turbo", "glm-5.1", "glm-5v-turbo"}

	for provider := range zaiProviders {
		for _, modelID := range modelIDs {
			t.Run(provider+"/"+modelID, func(t *testing.T) {
				candidate := normalize(modelID, source, providerRule{
					Provider: provider,
					Protocol: "openai-completions",
					Compat:   zaiCompat(),
				})

				if !reflect.DeepEqual(candidate.ThinkingLevelMap, wantLevels) {
					t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, wantLevels)
				}
				if candidate.Compat.SupportsReasoningEffort == nil || *candidate.Compat.SupportsReasoningEffort {
					t.Fatalf("SupportsReasoningEffort = %v, want false", candidate.Compat.SupportsReasoningEffort)
				}
				if candidate.Compat.ThinkingFormat != "zai" {
					t.Fatalf("ThinkingFormat = %q, want zai", candidate.Compat.ThinkingFormat)
				}
				if candidate.Compat.ZAIToolStream == nil || !*candidate.Compat.ZAIToolStream {
					t.Fatal("ZAI toggle must preserve zaiToolStream")
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

func TestNormalizeZAIEffortModelIsNotTreatedAsToggle(t *testing.T) {
	high, max := "high", "max"
	source := sourceModel{
		ToolCall:  true,
		Reasoning: true,
		ReasoningOptions: []sourceReasoningOption{{
			Type:   "effort",
			Values: []*string{&high, &max},
		}},
	}
	wantLevels := map[string]*string{
		"minimal": nil,
		"low":     &high,
		"medium":  &high,
		"high":    &high,
		"max":     &max,
	}

	for provider := range zaiProviders {
		t.Run(provider, func(t *testing.T) {
			models := []model{normalize("glm-5.2", source, providerRule{
				Provider: provider,
				Protocol: "openai-completions",
				Compat:   zaiCompat(),
			})}
			applyOverrides(models)
			candidate := models[0]

			if !reflect.DeepEqual(candidate.ThinkingLevelMap, wantLevels) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, wantLevels)
			}
			if candidate.Compat.SupportsReasoningEffort == nil || !*candidate.Compat.SupportsReasoningEffort {
				t.Fatal("GLM 5.2 must retain reasoning_effort support")
			}
		})
	}
}
