package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/llm"
)

// defaultThinkingBudgets maps a thinking level to a token budget for providers
// that use budget-based (non-adaptive) reasoning. It mirrors the reference
// defaults.
var defaultThinkingBudgets = map[llm.ModelThinkingLevel]int64{
	llm.ModelThinkingMinimal: 1024,
	llm.ModelThinkingLow:     2048,
	llm.ModelThinkingMedium:  8192,
	llm.ModelThinkingHigh:    16384,
	llm.ModelThinkingXHigh:   16384,
	llm.ModelThinkingMax:     16384,
}

// resolveThinkingLevel preserves the difference between an unspecified level
// and an explicit request, then clamps explicit levels to what the model
// supports.
func resolveThinkingLevel(model llm.Model, reasoning llm.ModelThinkingLevel) (llm.ModelThinkingLevel, bool) {
	if !model.Reasoning || reasoning == "" {
		return "", false
	}
	return llm.ClampThinkingLevel(model, reasoning), true
}

// thinkingActive reports whether the clamped request asks the model to reason.
// An empty level leaves the model default; a supported "off" disables thinking.
func thinkingActive(model llm.Model, reasoning llm.ModelThinkingLevel) bool {
	resolved, specified := resolveThinkingLevel(model, reasoning)
	return specified && resolved != llm.ModelThinkingOff
}

// applyThinking sets the reasoning request fields. Adaptive models receive
// thinking: adaptive plus an effort level; other reasoning models receive
// budget-based thinking. A supported "off" disables thinking, unsupported
// levels are clamped, and an empty level is left to the model's own default.
// display controls how thinking is returned and applies to both forms.
func applyThinking(params *sdk.MessageNewParams, model llm.Model, compat compat, reasoning llm.ModelThinkingLevel, display llm.ThinkingDisplay) {
	reasoning, specified := resolveThinkingLevel(model, reasoning)
	if !specified {
		return
	}
	if reasoning == llm.ModelThinkingOff {
		params.Thinking = sdk.ThinkingConfigParamUnion{OfDisabled: &sdk.ThinkingConfigDisabledParam{}}
		return
	}

	if compat.forceAdaptiveThinking {
		params.Thinking = sdk.ThinkingConfigParamUnion{
			OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{
				Display: adaptiveDisplay(display),
			},
		}
		if effort := mapEffort(model, reasoning); effort != "" {
			params.OutputConfig = sdk.OutputConfigParam{Effort: effort}
		}
		return
	}

	params.Thinking = sdk.ThinkingConfigParamUnion{
		OfEnabled: &sdk.ThinkingConfigEnabledParam{
			BudgetTokens: thinkingBudget(reasoning, params.MaxTokens),
			Display:      enabledDisplay(display),
		},
	}
}

// adaptiveDisplay maps the provider-independent display to the adaptive-thinking
// enum, defaulting to summarized so behavior matches the API default.
func adaptiveDisplay(display llm.ThinkingDisplay) sdk.ThinkingConfigAdaptiveDisplay {
	if display == llm.ThinkingDisplayOmitted {
		return sdk.ThinkingConfigAdaptiveDisplayOmitted
	}
	return sdk.ThinkingConfigAdaptiveDisplaySummarized
}

// enabledDisplay maps the provider-independent display to the budget-thinking
// enum, defaulting to summarized so behavior matches the API default.
func enabledDisplay(display llm.ThinkingDisplay) sdk.ThinkingConfigEnabledDisplay {
	if display == llm.ThinkingDisplayOmitted {
		return sdk.ThinkingConfigEnabledDisplayOmitted
	}
	return sdk.ThinkingConfigEnabledDisplaySummarized
}

// mapEffort maps a thinking level to an Anthropic effort, honoring an explicit
// per-model mapping when present.
func mapEffort(model llm.Model, level llm.ModelThinkingLevel) sdk.OutputConfigEffort {
	if mapped, ok := model.ThinkingLevelMap[level]; ok && mapped != nil {
		switch *mapped {
		case "low":
			return sdk.OutputConfigEffortLow
		case "medium":
			return sdk.OutputConfigEffortMedium
		case "high":
			return sdk.OutputConfigEffortHigh
		case "xhigh":
			return sdk.OutputConfigEffortXhigh
		case "max":
			return sdk.OutputConfigEffortMax
		}
	}
	switch level {
	case llm.ModelThinkingMinimal, llm.ModelThinkingLow:
		return sdk.OutputConfigEffortLow
	case llm.ModelThinkingMedium:
		return sdk.OutputConfigEffortMedium
	case llm.ModelThinkingXHigh:
		return sdk.OutputConfigEffortXhigh
	case llm.ModelThinkingMax:
		return sdk.OutputConfigEffortMax
	default:
		return sdk.OutputConfigEffortHigh
	}
}

// thinkingBudget returns a budget for budget-based thinking, kept strictly below
// max_tokens so the model retains room to answer.
func thinkingBudget(level llm.ModelThinkingLevel, maxTokens int64) int64 {
	budget, ok := defaultThinkingBudgets[level]
	if !ok {
		budget = defaultThinkingBudgets[llm.ModelThinkingMedium]
	}
	if maxTokens > 0 && budget >= maxTokens {
		budget = maxTokens - 1024
	}
	if budget < 1024 {
		budget = 1024
	}
	return budget
}
