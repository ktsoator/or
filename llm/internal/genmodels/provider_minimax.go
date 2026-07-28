package main

import "strings"

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
