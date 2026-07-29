package main

func normalizeDeepSeekModel(candidate *model, source sourceModel) {
	applyDeepSeekRequestCompatibility(candidate, source.ReasoningOptions)
}

// applyDeepSeekRequestCompatibility combines models.dev's route capabilities
// with the DeepSeek request dialect. Gateway routes keep separate profiles
// because an identical model ID does not guarantee identical wire behavior.
func applyDeepSeekRequestCompatibility(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || candidate.Provider != "deepseek" ||
		candidate.Protocol != "openai-completions" || !candidate.Reasoning {
		return
	}

	candidate.Compat.Kind = "openai"
	candidate.Compat.ThinkingFormat = "deepseek"
	candidate.Compat.RequiresReasoningContentOnAssistantMessages = boolp(true)

	levels := reasoningEffortLevels(options)
	hasToggle := hasReasoningOption(options, "toggle")
	switch {
	case len(levels) > 0:
		if hasToggle && !containsThinkingLevel(levels, "off") {
			levels = append([]string{"off"}, levels...)
		}
		applyThinkingProfile(candidate, effortThinking("deepseek", true, levels...))
	case hasToggle:
		applyThinkingProfile(candidate, toggleThinking("deepseek", true))
	}
}
