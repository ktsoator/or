package main

import "strings"

func applyOverrides(models []model) {
	for i := range models {
		m := &models[i]
		id := strings.ToLower(m.ID)
		applyKimiOverrides(m)
		applyMiniMaxOverrides(m)
		if m.Protocol == "anthropic-messages" && isAdaptiveAnthropic(m.ID) {
			m.Compat.Kind = "anthropic"
			m.Compat.ForceAdaptiveThinking = boolp(true)
			applyAdaptiveAnthropicThinkingLevels(m, id)
		}
		if m.Protocol == "anthropic-messages" && disablesAnthropicTemperature(id) {
			m.Compat.Kind = "anthropic"
			m.Compat.SupportsTemperature = boolp(false)
		}
		if strings.Contains(m.ID, "deepseek-v4") {
			high, max := "high", "max"
			m.ThinkingLevelMap = map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": &high, "max": &max}
			if m.Provider == "opencode" {
				m.ThinkingLevelMap["off"] = nil
			}
		}
		if m.Provider == "zai" || m.Provider == "zai-coding-cn" {
			if m.ID == "glm-5.2" {
				high, max := "high", "max"
				m.ThinkingLevelMap = map[string]*string{"minimal": nil, "low": &high, "medium": &high, "high": &high, "max": &max}
				m.Compat.SupportsReasoningEffort = boolp(true)
			}
		}
	}
}

func isAdaptiveAnthropic(id string) bool {
	id = strings.ToLower(id)
	return containsAny(id,
		"opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "opus-5", "opus.5",
		"sonnet-4-6", "sonnet-4.6", "sonnet-5", "sonnet.5", "fable-5",
	)
}

func applyAdaptiveAnthropicThinkingLevels(candidate *model, id string) {
	max, xhigh := "max", "xhigh"
	switch {
	case strings.Contains(id, "fable-5"):
		mergeThinkingLevelMap(candidate, map[string]*string{"off": nil, "xhigh": &xhigh, "max": &max})
	case containsAny(id,
		"opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "opus-5", "opus.5",
		"sonnet-5", "sonnet.5",
	):
		mergeThinkingLevelMap(candidate, map[string]*string{"xhigh": &xhigh, "max": &max})
	default:
		mergeThinkingLevelMap(candidate, map[string]*string{"max": &max})
	}
}

func mergeThinkingLevelMap(candidate *model, overrides map[string]*string) {
	if candidate.ThinkingLevelMap == nil {
		candidate.ThinkingLevelMap = make(map[string]*string, len(overrides))
	}
	for level, value := range overrides {
		candidate.ThinkingLevelMap[level] = value
	}
}

func disablesAnthropicTemperature(id string) bool {
	id = strings.ToLower(id)
	return containsAny(id, "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "opus-5", "opus.5")
}

func containsAny(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
