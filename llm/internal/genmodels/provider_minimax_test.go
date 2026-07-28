package main

import "testing"

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
