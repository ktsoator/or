package openai

import (
	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
)

// resolvedThinking preserves the difference between an omitted thinking option
// and an explicit request to disable thinking.
type resolvedThinking struct {
	specified bool
	level     llm.ModelThinkingLevel
}

func (thinking resolvedThinking) enabled() bool {
	return thinking.specified && thinking.level != llm.ModelThinkingOff
}

// resolveThinking clamps an explicitly requested level to one the model
// supports. An empty request stays unspecified so the provider keeps its own
// default instead of receiving a disable-thinking parameter.
func resolveThinking(model llm.Model, requested llm.ModelThinkingLevel) resolvedThinking {
	if requested == "" {
		return resolvedThinking{}
	}
	return resolvedThinking{specified: true, level: llm.ClampThinkingLevel(model, requested)}
}

// applyThinking sets the request fields that control reasoning, dispatching on
// the provider's thinking wire format. Unspecified requests return before any
// control field is written. Non-standard fields are written through
// SetExtraFields; reasoning_effort is carried that way too so all formats share
// one code path.
func applyThinking(
	params *oai.ChatCompletionNewParams,
	model llm.Model,
	compat resolvedCompat,
	thinking resolvedThinking,
) {
	if !model.Reasoning || !thinking.specified {
		return
	}
	hasEffort := thinking.enabled()
	effort := thinking.level
	extras := map[string]any{}

	switch compat.thinkingFormat {
	case "zai":
		extras["thinking"] = zaiThinkingType(hasEffort)
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
	case "xiaomi":
		applyXiaomiThinking(extras, thinking)
	case "deepseek":
		applyDeepSeekThinking(extras, model, compat, thinking)
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
		} else {
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

// applyXiaomiThinking writes Xiaomi's binary thinking toggle. Xiaomi does not
// accept OpenAI reasoning_effort levels on this request path.
func applyXiaomiThinking(extras map[string]any, thinking resolvedThinking) {
	extras["thinking"] = thinkingType(thinking.enabled())
}

// applyDeepSeekThinking writes DeepSeek's thinking toggle and, for routes that
// support it, the requested reasoning effort.
func applyDeepSeekThinking(
	extras map[string]any,
	model llm.Model,
	compat resolvedCompat,
	thinking resolvedThinking,
) {
	extras["thinking"] = thinkingType(thinking.enabled())
	if thinking.enabled() && compat.supportsReasoningEffort {
		extras["reasoning_effort"] = mappedEffort(model, thinking.level)
	}
}

func zaiThinkingType(enabled bool) map[string]any {
	if enabled {
		return map[string]any{"type": "enabled", "clear_thinking": false}
	}
	return thinkingType(false)
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
