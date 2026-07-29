package main

import (
	"reflect"
	"testing"
)

func TestNormalizeFireworksModelUsesOnlyVerifiedRoutes(t *testing.T) {
	high, max := "high", "max"
	options := []sourceReasoningOption{
		{Type: "toggle"},
		{Type: "effort", Values: []*string{&high, &max}},
	}

	for route := range fireworksVerifiedReasoningRoutes {
		t.Run(route, func(t *testing.T) {
			candidate := model{
				ID:        route,
				Provider:  "fireworks",
				Protocol:  "anthropic-messages",
				Reasoning: true,
			}
			normalizeFireworksModel(&candidate, sourceModel{ReasoningOptions: options})

			want := explicitThinkingLevels([]string{"off", "high", "max"})
			if !reflect.DeepEqual(candidate.ThinkingLevelMap, want) {
				t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, want)
			}
			if candidate.Compat.Kind != "" || candidate.Compat.ThinkingFormat != "" {
				t.Fatalf("Fireworks route inherited native model dialect: %#v", candidate.Compat)
			}
		})
	}

	tests := []model{
		{ID: "accounts/fireworks/models/deepseek-v4-lookalike", Provider: "fireworks", Protocol: "anthropic-messages", Reasoning: true},
		{ID: "accounts/fireworks/models/deepseek-v4-pro", Provider: "other", Protocol: "anthropic-messages", Reasoning: true},
	}
	for _, candidate := range tests {
		before := candidate
		normalizeFireworksModel(&candidate, sourceModel{ReasoningOptions: options})
		if !reflect.DeepEqual(candidate, before) {
			t.Fatalf("unverified route changed:\n got: %#v\nwant: %#v", candidate, before)
		}
	}
}
