package main

import "fmt"

type modelRouteKey struct {
	// Provider identifies the configured endpoint as well as the vendor route.
	Provider string
	ModelID  string
}

type thinkingControl string

const (
	thinkingNone   thinkingControl = "none"
	thinkingFixed  thinkingControl = "fixed"
	thinkingToggle thinkingControl = "toggle"
	thinkingEffort thinkingControl = "effort"
)

type thinkingProfile struct {
	Control         thinkingControl
	Format          string
	Levels          []string
	Visibility      string
	ReplayReasoning bool
}

func fixedThinking(visibility string, replayReasoning bool) thinkingProfile {
	return thinkingProfile{
		Control:         thinkingFixed,
		Visibility:      visibility,
		ReplayReasoning: replayReasoning,
	}
}

func fixedProviderDefault() thinkingProfile {
	return fixedThinking("", false)
}

func toggleThinking(format string, replayReasoning bool) thinkingProfile {
	return thinkingProfile{
		Control:         thinkingToggle,
		Format:          format,
		Levels:          []string{"off", "high"},
		ReplayReasoning: replayReasoning,
	}
}

func effortThinking(format string, replayReasoning bool, levels ...string) thinkingProfile {
	return thinkingProfile{
		Control:         thinkingEffort,
		Format:          format,
		Levels:          append([]string(nil), levels...),
		ReplayReasoning: replayReasoning,
	}
}

func applyThinkingProfile(candidate *model, profile thinkingProfile) {
	if candidate == nil {
		return
	}

	candidate.ThinkingLevelMap = nil
	candidate.ThinkingVisibility = profile.Visibility
	candidate.Compat.ThinkingFormat = profile.Format
	candidate.Compat.RequiresReasoningContentOnAssistantMessages = nil
	if profile.ReplayReasoning {
		candidate.Compat.RequiresReasoningContentOnAssistantMessages = boolp(true)
	}

	switch profile.Control {
	case thinkingNone:
		candidate.Reasoning = false
		candidate.Compat.SupportsReasoningEffort = boolp(false)
	case thinkingFixed:
		candidate.Compat.SupportsReasoningEffort = boolp(false)
		candidate.ThinkingLevelMap = unsupportedThinkingLevels("off", "minimal", "low", "medium")
	case thinkingToggle:
		candidate.Compat.SupportsReasoningEffort = boolp(false)
		candidate.ThinkingLevelMap = unsupportedThinkingLevels("minimal", "low", "medium")
	case thinkingEffort:
		candidate.Compat.SupportsReasoningEffort = boolp(true)
		candidate.ThinkingLevelMap = explicitThinkingLevels(profile.Levels)
	}
}

func unsupportedThinkingLevels(levels ...string) map[string]*string {
	result := make(map[string]*string, len(levels))
	for _, level := range levels {
		result[level] = nil
	}
	return result
}

func explicitThinkingLevels(levels []string) map[string]*string {
	result := unsupportedThinkingLevels(generatedThinkingLevels...)
	for _, level := range levels {
		value := level
		if level == "off" {
			value = "none"
		}
		result[level] = stringPointer(value)
	}
	return result
}

func validateThinkingProfiles(profiles map[modelRouteKey]thinkingProfile) error {
	for key, profile := range profiles {
		if err := validateThinkingProfile(key, profile); err != nil {
			return err
		}
	}
	return nil
}

func validateThinkingProfile(key modelRouteKey, profile thinkingProfile) error {
	if key.Provider == "" || key.ModelID == "" {
		return fmt.Errorf("thinking profile has incomplete route identity")
	}
	if profile.Visibility != "" && profile.Visibility != "visible" && profile.Visibility != "hidden" {
		return fmt.Errorf("thinking profile %s/%s has invalid visibility %q", key.Provider, key.ModelID, profile.Visibility)
	}

	switch profile.Control {
	case thinkingNone:
		if profile.Format != "" || len(profile.Levels) != 0 || profile.ReplayReasoning {
			return fmt.Errorf("thinking profile %s/%s disables reasoning but still declares behavior", key.Provider, key.ModelID)
		}
	case thinkingFixed:
		if profile.Format != "" || len(profile.Levels) != 0 {
			return fmt.Errorf("fixed thinking profile %s/%s must not declare controls", key.Provider, key.ModelID)
		}
	case thinkingToggle:
		if profile.Format == "" {
			return fmt.Errorf("toggle thinking profile %s/%s has no wire format", key.Provider, key.ModelID)
		}
		if !isKnownThinkingFormat(profile.Format) {
			return fmt.Errorf("toggle thinking profile %s/%s has unknown wire format %q", key.Provider, key.ModelID, profile.Format)
		}
		if !sameThinkingLevels(profile.Levels, []string{"off", "high"}) {
			return fmt.Errorf("toggle thinking profile %s/%s must expose only off and high", key.Provider, key.ModelID)
		}
	case thinkingEffort:
		if len(profile.Levels) == 0 {
			return fmt.Errorf("effort thinking profile %s/%s has no levels", key.Provider, key.ModelID)
		}
		if profile.Format != "" && !isKnownThinkingFormat(profile.Format) {
			return fmt.Errorf("effort thinking profile %s/%s has unknown wire format %q", key.Provider, key.ModelID, profile.Format)
		}
		seen := make(map[string]struct{}, len(profile.Levels))
		for _, level := range profile.Levels {
			if !isGeneratedThinkingLevel(level) {
				return fmt.Errorf("effort thinking profile %s/%s has invalid level %q", key.Provider, key.ModelID, level)
			}
			if _, duplicate := seen[level]; duplicate {
				return fmt.Errorf("effort thinking profile %s/%s repeats level %q", key.Provider, key.ModelID, level)
			}
			seen[level] = struct{}{}
		}
	default:
		return fmt.Errorf("thinking profile %s/%s has invalid control %q", key.Provider, key.ModelID, profile.Control)
	}
	return nil
}

func validateAppliedThinkingProfile(candidate model, profile thinkingProfile) error {
	key := modelRouteKey{Provider: candidate.Provider, ModelID: candidate.ID}
	if profile.Control != thinkingNone && !candidate.Reasoning {
		return fmt.Errorf("generated catalog model %s/%s has a thinking profile but reasoning is disabled", key.Provider, key.ModelID)
	}
	if candidate.Protocol != "openai-completions" {
		return fmt.Errorf("generated catalog model %s/%s has an OpenAI thinking profile on protocol %q", key.Provider, key.ModelID, candidate.Protocol)
	}
	if candidate.Compat.Kind != "openai" {
		return fmt.Errorf("generated catalog model %s/%s has no OpenAI compatibility data", key.Provider, key.ModelID)
	}

	expected := model{Reasoning: candidate.Reasoning, Compat: compatibility{Kind: candidate.Compat.Kind}}
	applyThinkingProfile(&expected, profile)
	if candidate.Reasoning != expected.Reasoning ||
		candidate.Compat.ThinkingFormat != expected.Compat.ThinkingFormat ||
		!sameBoolPointer(candidate.Compat.SupportsReasoningEffort, expected.Compat.SupportsReasoningEffort) ||
		!sameBoolPointer(candidate.Compat.RequiresReasoningContentOnAssistantMessages, expected.Compat.RequiresReasoningContentOnAssistantMessages) ||
		candidate.ThinkingVisibility != expected.ThinkingVisibility ||
		!sameThinkingLevelMap(candidate.ThinkingLevelMap, expected.ThinkingLevelMap) {
		return fmt.Errorf("generated catalog model %s/%s does not match its thinking profile", key.Provider, key.ModelID)
	}
	return nil
}

func sameBoolPointer(got, want *bool) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func sameThinkingLevelMap(got, want map[string]*string) bool {
	if len(got) != len(want) {
		return false
	}
	for level, wantValue := range want {
		gotValue, ok := got[level]
		if !ok || (gotValue == nil) != (wantValue == nil) {
			return false
		}
		if gotValue != nil && *gotValue != *wantValue {
			return false
		}
	}
	return true
}

func sameThinkingLevels(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(got))
	for _, level := range got {
		seen[level] = struct{}{}
	}
	for _, level := range want {
		if _, ok := seen[level]; !ok {
			return false
		}
	}
	return true
}

func isGeneratedThinkingLevel(candidate string) bool {
	for _, level := range generatedThinkingLevels {
		if candidate == level {
			return true
		}
	}
	return false
}

func isKnownThinkingFormat(candidate string) bool {
	switch candidate {
	case "openai", "deepseek", "together", "zai", "qwen", "qwen-chat-template", "string-thinking", "ant-ling":
		return true
	default:
		return false
	}
}
