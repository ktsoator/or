package main

import (
	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/internal/openaicompat"
)

var generatedThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

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

// supportsDirectReasoningEffort delegates to the runtime compatibility
// resolver. Non-standard thinking formats and explicit opt-outs remain under
// their provider rules instead of accepting generic models.dev effort values.
func supportsDirectReasoningEffort(candidate model) bool {
	return openaicompat.SupportsDirectReasoningEffort(runtimeCompatibilityModel(candidate))
}

// runtimeCompatibilityModel translates generator data into the SDK model used
// by the request adapter. Keeping this conversion here makes generation ask the
// same resolver that will build the eventual request.
func runtimeCompatibilityModel(candidate model) llm.Model {
	result := llm.Model{
		ID:       candidate.ID,
		Provider: candidate.Provider,
		Protocol: llm.Protocol(candidate.Protocol),
		BaseURL:  candidate.BaseURL,
	}
	if candidate.Compat.Kind != "openai" {
		return result
	}
	result.Compatibility = &llm.OpenAICompletionsCompatibility{
		SupportsStore:                               candidate.Compat.SupportsStore,
		SupportsDeveloperRole:                       candidate.Compat.SupportsDeveloperRole,
		SupportsReasoningEffort:                     candidate.Compat.SupportsReasoningEffort,
		MaxTokensField:                              candidate.Compat.MaxTokensField,
		SupportsStrictMode:                          candidate.Compat.SupportsStrictMode,
		RequiresReasoningContentOnAssistantMessages: candidate.Compat.RequiresReasoningContentOnAssistantMessages,
		RequiresThinkingAsText:                      candidate.Compat.RequiresThinkingAsText,
		ThinkingFormat:                              candidate.Compat.ThinkingFormat,
		ZAIToolStream:                               candidate.Compat.ZAIToolStream,
	}
	return result
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
	return levelMap
}

func stringPointer(value string) *string { return &value }
