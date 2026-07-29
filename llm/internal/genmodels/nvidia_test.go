package main

import (
	"reflect"
	"testing"
)

func TestNormalizeNVIDIAModelUsesOnlyVerifiedRoutes(t *testing.T) {
	none, high, max := "none", "high", "max"
	options := []sourceReasoningOption{
		{Type: "effort", Values: []*string{&none, &high, &max}},
	}

	for route := range nvidiaVerifiedReasoningRoutes {
		t.Run(route, func(t *testing.T) {
			candidate := model{
				ID:        route,
				Provider:  "nvidia",
				Protocol:  "openai-completions",
				Reasoning: true,
			}
			normalizeNVIDIAModel(&candidate, sourceModel{ReasoningOptions: options})

			want := explicitThinkingLevels([]string{"off", "high", "max"})
			if !reflect.DeepEqual(candidate.ThinkingLevelMap, want) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, want)
			}
			if candidate.Compat.Kind != "" || candidate.Compat.ThinkingFormat != "" {
				t.Fatalf("NVIDIA route inherited native model dialect: %#v", candidate.Compat)
			}
		})
	}

	tests := []model{
		{ID: "deepseek-ai/deepseek-v4-lookalike", Provider: "nvidia", Protocol: "openai-completions", Reasoning: true},
		{ID: "deepseek-ai/deepseek-v4-pro", Provider: "other", Protocol: "openai-completions", Reasoning: true},
	}
	for _, candidate := range tests {
		before := candidate
		normalizeNVIDIAModel(&candidate, sourceModel{ReasoningOptions: options})
		if !reflect.DeepEqual(candidate, before) {
			t.Fatalf("unverified route changed:\n got: %#v\nwant: %#v", candidate, before)
		}
	}
}
