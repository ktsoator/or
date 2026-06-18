package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/ktsoator/or/internal/llm"
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
}

// thinkingActive reports whether the request asks the model to reason. An empty
// level leaves the model default; "off" disables thinking explicitly.
func thinkingActive(model llm.Model, reasoning llm.ModelThinkingLevel) bool {
	return model.Reasoning && reasoning != "" && reasoning != llm.ModelThinkingOff
}

// applyThinking sets the reasoning request fields. Adaptive models receive
// thinking: adaptive plus an effort level; other reasoning models receive
// budget-based thinking. "off" disables thinking; an empty level is left to the
// model's own default.
func applyThinking(params *sdk.MessageNewParams, model llm.Model, compat compat, reasoning llm.ModelThinkingLevel) {
	if !model.Reasoning || reasoning == "" {
		return
	}
	if reasoning == llm.ModelThinkingOff {
		params.Thinking = sdk.ThinkingConfigParamUnion{OfDisabled: &sdk.ThinkingConfigDisabledParam{}}
		return
	}

	if compat.forceAdaptiveThinking {
		params.Thinking = sdk.ThinkingConfigParamUnion{
			OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{
				Display: sdk.ThinkingConfigAdaptiveDisplaySummarized,
			},
		}
		if effort := mapEffort(model, reasoning); effort != "" {
			params.OutputConfig = sdk.OutputConfigParam{Effort: effort}
		}
		return
	}

	params.Thinking = sdk.ThinkingConfigParamUnion{
		OfEnabled: &sdk.ThinkingConfigEnabledParam{BudgetTokens: thinkingBudget(reasoning, params.MaxTokens)},
	}
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
		}
	}
	switch level {
	case llm.ModelThinkingMinimal, llm.ModelThinkingLow:
		return sdk.OutputConfigEffortLow
	case llm.ModelThinkingMedium:
		return sdk.OutputConfigEffortMedium
	case llm.ModelThinkingXHigh:
		return sdk.OutputConfigEffortXhigh
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
