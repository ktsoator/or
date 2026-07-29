package main

var xiaomiProviders = map[string]struct{}{
	"xiaomi":                {},
	"xiaomi-token-plan-cn":  {},
	"xiaomi-token-plan-ams": {},
	"xiaomi-token-plan-sgp": {},
}

// applyXiaomiRequestCompatibility compiles Xiaomi's route-specific toggle
// metadata into the runtime thinking contract. Xiaomi uses the DeepSeek-style
// thinking object and does not accept named reasoning_effort levels.
func applyXiaomiRequestCompatibility(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || !candidate.Reasoning || candidate.Protocol != "openai-completions" {
		return
	}
	if _, ok := xiaomiProviders[candidate.Provider]; !ok || !hasOnlyReasoningOption(options, "toggle") {
		return
	}
	applyThinkingProfile(candidate, toggleThinking("deepseek", true))
}
