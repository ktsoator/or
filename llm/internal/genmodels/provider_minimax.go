package main

import "strings"

// applyMiniMaxThinkingMetadata narrows models.dev's pure toggle to one off and
// one on state. MiniMax still uses Anthropic's budget-thinking wire format;
// "high" selects the existing fixed high budget rather than a named effort.
func applyMiniMaxThinkingMetadata(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || !candidate.Reasoning || candidate.Protocol != "anthropic-messages" ||
		!hasOnlyReasoningOption(options, "toggle") {
		return
	}

	provider := strings.ToLower(candidate.Provider)
	id := strings.ToLower(candidate.ID)
	if provider != "minimax" && provider != "minimax-cn" &&
		(provider != "opencode-go" || !strings.HasPrefix(id, "minimax-")) {
		return
	}

	candidate.ThinkingLevelMap = unsupportedThinkingLevels("minimal", "low", "medium")
}

// applyMiniMaxOverrides fills capability gaps in public model catalogs. Direct
// MiniMax M2.7 endpoints do not support disabling thinking, while M3 does.
func applyMiniMaxOverrides(candidate *model) {
	if candidate == nil || candidate.Protocol != "anthropic-messages" {
		return
	}
	provider := strings.ToLower(candidate.Provider)
	if provider != "minimax" && provider != "minimax-cn" {
		return
	}

	switch strings.ToLower(candidate.ID) {
	case "minimax-m2.7", "minimax-m2.7-highspeed":
		mergeThinkingLevelMap(candidate, map[string]*string{"off": nil})
	}
}
