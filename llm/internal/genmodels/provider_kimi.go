package main

import "strings"

var moonshotProviders = map[string]struct{}{
	"moonshotai":    {},
	"moonshotai-cn": {},
}

// applyMoonshotThinkingCompatibility compiles models.dev's pure toggle into
// Moonshot's DeepSeek-style thinking object. Models that also declare effort,
// such as Kimi K3, stay on their dedicated effort path.
func applyMoonshotThinkingCompatibility(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || !candidate.Reasoning || candidate.Protocol != "openai-completions" {
		return
	}
	if _, ok := moonshotProviders[candidate.Provider]; !ok || !hasOnlyReasoningOption(options, "toggle") {
		return
	}
	applyThinkingProfile(candidate, toggleThinking("deepseek", false))
}

func applyKimiRequestCompatibility(candidate *model) {
	if candidate == nil {
		return
	}
	provider := strings.ToLower(candidate.Provider)
	id := strings.ToLower(candidate.ID)

	if provider == "kimi-coding" && candidate.Protocol == "anthropic-messages" {
		candidate.Compat.Kind = "anthropic"
		candidate.Compat.ForceAdaptiveThinking = boolp(true)
	}
	if (provider == "moonshotai" || provider == "moonshotai-cn") &&
		id == "kimi-k3" && candidate.Protocol == "openai-completions" {
		candidate.Compat.Kind = "openai"
		candidate.Compat.ThinkingFormat = ""
		candidate.Compat.SupportsReasoningEffort = boolp(true)
		candidate.Compat.RequiresReasoningContentOnAssistantMessages = boolp(true)
	}
}

func applyKimiOverrides(candidate *model) {
	if candidate == nil {
		return
	}
	applyKimiRequestCompatibility(candidate)
	provider := strings.ToLower(candidate.Provider)
	id := strings.ToLower(candidate.ID)

	if provider == "kimi-coding" && candidate.Protocol == "anthropic-messages" {
		candidate.Compat.AllowEmptySignature = nil
		switch id {
		case "k3", "kimi-for-coding":
			candidate.Compat.AllowEmptySignature = boolp(true)
		}
	}

	if (provider == "moonshotai" || provider == "moonshotai-cn") &&
		(id == "kimi-k2.7-code" || id == "kimi-k2.7-code-highspeed") {
		mergeThinkingLevelMap(candidate, map[string]*string{"off": nil})
	}
}
