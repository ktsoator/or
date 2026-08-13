package protocolutil

import "github.com/ktsoator/or/llm"

// Thinking preserves the difference between an omitted thinking option and an
// explicit request to disable thinking.
type Thinking struct {
	Specified bool
	Level     llm.ModelThinkingLevel
}

func (thinking Thinking) Enabled() bool {
	return thinking.Specified && thinking.Level != llm.ModelThinkingOff
}

// ResolveThinking clamps an explicitly requested level to one the model
// supports. An empty request stays unspecified.
func ResolveThinking(model llm.Model, requested llm.ModelThinkingLevel) Thinking {
	if requested == "" {
		return Thinking{}
	}
	return Thinking{Specified: true, Level: llm.ClampThinkingLevel(model, requested)}
}

// MappedEffort returns the provider-specific value for a level, falling back to
// the level's own name when the model omits a mapping.
func MappedEffort(model llm.Model, level llm.ModelThinkingLevel) string {
	if value, ok := model.ThinkingLevelMap[level]; ok && value != nil {
		return *value
	}
	return string(level)
}

// ExplicitMappedEffort returns an effort only when the model catalog explicitly
// maps the level. A supported but absent mapping represents the provider's fixed
// default and must not be serialized as a tunable effort.
func ExplicitMappedEffort(model llm.Model, level llm.ModelThinkingLevel) (string, bool) {
	value, ok := model.ThinkingLevelMap[level]
	if !ok || value == nil {
		return "", false
	}
	return *value, true
}

// OffEffort returns the provider value for disabled thinking.
func OffEffort(model llm.Model) string {
	if value, ok := model.ThinkingLevelMap[llm.ModelThinkingOff]; ok && value != nil {
		return *value
	}
	return "none"
}

// OffString returns the model's explicit disabled mapping, if any.
func OffString(model llm.Model) (string, bool) {
	if value, ok := model.ThinkingLevelMap[llm.ModelThinkingOff]; ok && value != nil {
		return *value, true
	}
	return "", false
}
