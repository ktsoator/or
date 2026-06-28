package openai

import (
	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
)

// resolveEffort clamps a requested thinking level to one the model supports and
// returns it, or "" when thinking is unset or resolves to off.
func resolveEffort(model llm.Model, requested llm.ModelThinkingLevel) llm.ModelThinkingLevel {
	if requested == "" {
		return ""
	}
	clamped := llm.ClampThinkingLevel(model, requested)
	if clamped == llm.ModelThinkingOff {
		return ""
	}
	return clamped
}

// applyThinking sets the request fields that control reasoning, dispatching on
// the provider's thinking wire format. effort is the clamped level ("" = off).
// Non-standard fields are written through SetExtraFields; reasoning_effort is
// carried that way too so all formats share one code path.
func applyThinking(
	params *oai.ChatCompletionNewParams,
	model llm.Model,
	compat resolvedCompat,
	effort llm.ModelThinkingLevel,
) {
	if !model.Reasoning {
		return
	}
	hasEffort := effort != ""
	extras := map[string]any{}

	switch compat.thinkingFormat {
	case "zai":
		extras["thinking"] = thinkingType(hasEffort)
		if hasEffort && compat.supportsReasoningEffort {
			extras["reasoning_effort"] = mappedEffort(model, effort)
		}
	case "qwen":
		extras["enable_thinking"] = hasEffort
	case "qwen-chat-template":
		extras["chat_template_kwargs"] = map[string]any{
			"enable_thinking":   hasEffort,
			"preserve_thinking": true,
		}
	case "deepseek":
		if hasEffort {
			extras["thinking"] = thinkingType(true)
		} else if !offIsNull(model) {
			extras["thinking"] = thinkingType(false)
		}
		if hasEffort && compat.supportsReasoningEffort {
			extras["reasoning_effort"] = mappedEffort(model, effort)
		}
	case "openrouter":
		if hasEffort {
			extras["reasoning"] = map[string]any{"effort": mappedEffort(model, effort)}
		} else if !offIsNull(model) {
			extras["reasoning"] = map[string]any{"effort": offEffort(model)}
		}
	case "ant-ling":
		// ant-ling only sends reasoning when the level is explicitly mapped.
		if hasEffort {
			if value, ok := model.ThinkingLevelMap[effort]; ok && value != nil {
				extras["reasoning"] = map[string]any{"effort": *value}
			}
		}
	case "together":
		extras["reasoning"] = map[string]any{"enabled": hasEffort}
		if hasEffort && compat.supportsReasoningEffort {
			extras["reasoning_effort"] = mappedEffort(model, effort)
		}
	case "string-thinking":
		if hasEffort {
			extras["thinking"] = mappedEffort(model, effort)
		} else if !offIsNull(model) {
			extras["thinking"] = offEffort(model)
		}
	default: // "openai"
		if compat.supportsReasoningEffort {
			if hasEffort {
				extras["reasoning_effort"] = mappedEffort(model, effort)
			} else if value, ok := offString(model); ok {
				extras["reasoning_effort"] = value
			}
		}
	}

	if len(extras) > 0 {
		mergeExtraFields(params, extras)
	}
}

func thinkingType(enabled bool) map[string]any {
	if enabled {
		return map[string]any{"type": "enabled"}
	}
	return map[string]any{"type": "disabled"}
}

// mappedEffort returns the provider-specific value for a level, falling back to
// the level's own name when the model maps it to nil or omits it.
func mappedEffort(model llm.Model, level llm.ModelThinkingLevel) string {
	if value, ok := model.ThinkingLevelMap[level]; ok && value != nil {
		return *value
	}
	return string(level)
}

// offEffort returns the provider value for the disabled state, defaulting to
// "none" when the model does not map "off" to a concrete value.
func offEffort(model llm.Model) string {
	if value, ok := model.ThinkingLevelMap[llm.ModelThinkingOff]; ok && value != nil {
		return *value
	}
	return "none"
}

// offString returns the model's explicit "off" mapping, if any.
func offString(model llm.Model) (string, bool) {
	if value, ok := model.ThinkingLevelMap[llm.ModelThinkingOff]; ok && value != nil {
		return *value, true
	}
	return "", false
}

// offIsNull reports whether the model explicitly maps "off" to nil, marking the
// disabled state as unsupported (so no "disable thinking" field is sent).
func offIsNull(model llm.Model) bool {
	value, ok := model.ThinkingLevelMap[llm.ModelThinkingOff]
	return ok && value == nil
}
