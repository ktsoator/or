package main

import "strings"

func applyOverrides(models []model) {
	for i := range models {
		m := &models[i]
		if m.Protocol == "anthropic-messages" && isAdaptiveAnthropic(m.ID) {
			m.Compat.Kind = "anthropic"
			m.Compat.ForceAdaptiveThinking = boolp(true)
		}
		id := strings.ToLower(m.ID)
		if m.Protocol == "anthropic-messages" && disablesAnthropicTemperature(id) {
			m.Compat.Kind = "anthropic"
			m.Compat.SupportsTemperature = boolp(false)
		}
		if strings.Contains(m.ID, "deepseek-v4") {
			high, max := "high", "max"
			m.ThinkingLevelMap = map[string]*string{"minimal": nil, "low": nil, "medium": nil, "high": &high, "xhigh": &max}
		}
		if m.Provider == "zai" || m.Provider == "zai-coding-cn" {
			if m.ID == "glm-5.2" {
				high, max := "high", "max"
				m.ThinkingLevelMap = map[string]*string{"minimal": nil, "low": &high, "medium": &high, "high": &high, "xhigh": &max}
				m.Compat.SupportsReasoningEffort = boolp(true)
			}
		}
	}
}

func isAdaptiveAnthropic(id string) bool {
	id = strings.ToLower(id)
	for _, marker := range []string{"opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "opus-5", "sonnet-4-6", "sonnet-4.6", "fable-5"} {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

func disablesAnthropicTemperature(id string) bool {
	id = strings.ToLower(id)
	for _, marker := range []string{"opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "opus-5", "fable-5"} {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}
