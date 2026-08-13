package main

import (
	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/internal/openaicompat"
)

var generatedThinkingLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

func hasOnlyReasoningOption(options []sourceReasoningOption, optionType string) bool {
	return len(options) == 1 && options[0].Type == optionType
}

func hasReasoningOption(options []sourceReasoningOption, optionType string) bool {
	for _, option := range options {
		if option.Type == optionType {
			return true
		}
	}
	return false
}

func reasoningEffortLevels(options []sourceReasoningOption) []string {
	var levels []string
	for _, option := range options {
		if option.Type != "effort" {
			continue
		}
		for _, value := range option.Values {
			if value == nil {
				continue
			}
			level := *value
			if level == "none" {
				level = "off"
			}
			if !isGeneratedThinkingLevel(level) || containsThinkingLevel(levels, level) {
				continue
			}
			levels = append(levels, level)
		}
	}
	return levels
}

func containsThinkingLevel(levels []string, candidate string) bool {
	for _, level := range levels {
		if level == candidate {
			return true
		}
	}
	return false
}

// applyVerifiedGatewayReasoningMetadata preserves source-declared controls for
// an exact gateway route without assigning the native model vendor's wire
// dialect. The owning provider normalizer decides which routes are verified.
func applyVerifiedGatewayReasoningMetadata(candidate *model, options []sourceReasoningOption) {
	if candidate == nil {
		return
	}
	levels := reasoningEffortLevels(options)
	if hasReasoningOption(options, "toggle") && !containsThinkingLevel(levels, "off") {
		levels = append([]string{"off"}, levels...)
	}
	if len(levels) > 0 {
		candidate.ThinkingLevelMap = explicitThinkingLevels(levels)
	}
}

// applyReasoningOptionMetadata converts verified models.dev effort values for
// models whose wire protocol accepts named effort levels. Toggle and
// token-budget controls belong to their protocol-specific compatibility rules.
func applyReasoningOptionMetadata(candidate *model, options []sourceReasoningOption) {
	if candidate == nil || !supportsEffortThinkingLevels(*candidate) {
		return
	}
	if levelMap := effortThinkingLevelMap(options); levelMap != nil {
		candidate.ThinkingLevelMap = levelMap
	}
}

// supportsEffortThinkingLevels accepts standard OpenAI reasoning_effort and
// Anthropic-compatible adaptive thinking. Other thinking formats remain under
// their provider rules instead of accepting generic models.dev effort values.
func supportsEffortThinkingLevels(candidate model) bool {
	if candidate.Protocol == "openai-responses" {
		return true
	}
	if candidate.Protocol == "anthropic-messages" {
		return candidate.Compat.ForceAdaptiveThinking != nil && *candidate.Compat.ForceAdaptiveThinking
	}
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
