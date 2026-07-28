package main

var togetherReasoningOnlyModels = map[string]struct{}{
	"deepseek-ai/DeepSeek-R1": {},
	"MiniMaxAI/MiniMax-M2.7":  {},
}

var togetherReasoningEffortModels = map[string]struct{}{
	"openai/gpt-oss-20b":  {},
	"openai/gpt-oss-120b": {},
}

var togetherToggleReasoningEffortModels = map[string]struct{}{
	"deepseek-ai/DeepSeek-V4-Pro": {},
}

// applyTogetherRequestCompatibility classifies Together models by the
// reasoning controls accepted by their routed backend. The endpoint alone is
// insufficient because Together mixes fixed, toggle, and effort-based models.
func applyTogetherRequestCompatibility(candidate *model) {
	if candidate == nil || candidate.Provider != "together" || candidate.Protocol != "openai-completions" {
		return
	}

	candidate.Compat.Kind = "openai"
	candidate.Compat.SupportsStore = boolp(false)
	candidate.Compat.SupportsDeveloperRole = boolp(false)
	candidate.Compat.SupportsReasoningEffort = boolp(false)
	candidate.Compat.MaxTokensField = "max_tokens"
	candidate.Compat.SupportsStrictMode = boolp(false)
	if !candidate.Reasoning {
		return
	}

	switch {
	case togetherContainsModel(togetherReasoningEffortModels, candidate.ID):
		candidate.Compat.SupportsReasoningEffort = boolp(true)
		candidate.Compat.ThinkingFormat = "openai"
		candidate.ThinkingLevelMap = togetherUnsupportedThinkingLevels("off", "minimal")
	case togetherContainsModel(togetherToggleReasoningEffortModels, candidate.ID):
		candidate.Compat.SupportsReasoningEffort = boolp(true)
		candidate.Compat.ThinkingFormat = "together"
		candidate.ThinkingLevelMap = togetherUnsupportedThinkingLevels("minimal", "low", "medium", "xhigh")
	case togetherContainsModel(togetherReasoningOnlyModels, candidate.ID):
		candidate.Compat.ThinkingFormat = "openai"
		candidate.ThinkingLevelMap = togetherUnsupportedThinkingLevels("off", "minimal", "low", "medium")
	default:
		candidate.Compat.ThinkingFormat = "together"
		candidate.ThinkingLevelMap = togetherUnsupportedThinkingLevels("minimal", "low", "medium")
	}
}

func togetherContainsModel(models map[string]struct{}, id string) bool {
	_, ok := models[id]
	return ok
}

func togetherUnsupportedThinkingLevels(levels ...string) map[string]*string {
	result := make(map[string]*string, len(levels))
	for _, level := range levels {
		result[level] = nil
	}
	return result
}
