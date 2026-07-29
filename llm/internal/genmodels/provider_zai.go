package main

var zaiProviders = map[string]struct{}{
	"zai":           {},
	"zai-coding-cn": {},
}

// applyZAIThinkingCompatibility compiles models.dev's pure toggle into Z.AI's
// thinking object. Effort-capable models remain on their separate mapping.
func applyZAIThinkingCompatibility(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || !candidate.Reasoning || candidate.Protocol != "openai-completions" {
		return
	}
	if _, ok := zaiProviders[candidate.Provider]; !ok || !hasOnlyReasoningOption(options, "toggle") {
		return
	}
	applyThinkingProfile(candidate, toggleThinking("zai", false))
}
