package main

var generatedThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh"}

// applyReasoningOptionMetadata converts verified models.dev effort values only
// for models that use the standard OpenAI reasoning_effort field. Toggle and
// token-budget controls belong to their protocol-specific compatibility rules.
func applyReasoningOptionMetadata(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || !supportsDirectReasoningEffort(*candidate) {
		return
	}
	if levelMap := effortThinkingLevelMap(options); levelMap != nil {
		candidate.ThinkingLevelMap = levelMap
	}
}

// supportsDirectReasoningEffort is deliberately conservative. Generator
// routes opt in by using OpenAI compatibility; non-standard thinking formats
// and explicit opt-outs must be handled by provider rules instead.
func supportsDirectReasoningEffort(candidate model) bool {
	if candidate.Protocol != "openai-completions" || candidate.Compat.Kind != "openai" {
		return false
	}
	if candidate.Compat.ThinkingFormat != "" && candidate.Compat.ThinkingFormat != "openai" {
		return false
	}
	return candidate.Compat.SupportsReasoningEffort == nil || *candidate.Compat.SupportsReasoningEffort
}

func effortThinkingLevelMap(options []sourceReasoningOption) map[string]*string {
	supported := make(map[string]bool)
	for _, option := range options {
		if option.Type != "effort" {
			continue
		}
		for _, value := range option.Values {
			if value == nil {
				continue
			}
			switch *value {
			case "none", "minimal", "low", "medium", "high", "xhigh", "max":
				supported[*value] = true
			}
		}
	}
	if len(supported) == 0 {
		return nil
	}

	levelMap := make(map[string]*string, len(generatedThinkingLevels))
	for _, level := range generatedThinkingLevels {
		levelMap[level] = nil
	}
	if supported["none"] {
		levelMap["off"] = stringPointer("none")
	}
	for _, level := range generatedThinkingLevels[1:] {
		if supported[level] {
			levelMap[level] = stringPointer(level)
		}
	}
	if !supported["xhigh"] && supported["max"] {
		levelMap["xhigh"] = stringPointer("max")
	}
	return levelMap
}

func stringPointer(value string) *string { return &value }
