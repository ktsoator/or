package main

import "strings"

type openCodeVariant struct {
	Source   string
	Provider string
	BaseURL  string
}

var openCodeVariants = []openCodeVariant{
	{Source: "opencode", Provider: "opencode", BaseURL: "https://opencode.ai/zen"},
	{Source: "opencode-go", Provider: "opencode-go", BaseURL: "https://opencode.ai/zen/go"},
}

var openCodeThinkingProfiles = map[modelRouteKey]thinkingProfile{
	{Provider: "opencode", ModelID: "deepseek-v4-flash"}:      effortThinking("", true, "high", "max"),
	{Provider: "opencode", ModelID: "deepseek-v4-flash-free"}: effortThinking("", true, "high", "max"),
	{Provider: "opencode", ModelID: "deepseek-v4-pro"}:        effortThinking("", true, "high", "max"),
	{Provider: "opencode", ModelID: "kimi-k2.6"}:              toggleThinking("deepseek", false),

	{Provider: "opencode-go", ModelID: "deepseek-v4-flash"}: effortThinking("deepseek", true, "off", "high", "max"),
	{Provider: "opencode-go", ModelID: "deepseek-v4-pro"}:   effortThinking("deepseek", true, "off", "high", "max"),
	{Provider: "opencode-go", ModelID: "glm-5.2"}:           effortThinking("", false, "high", "max"),
	{Provider: "opencode-go", ModelID: "kimi-k2.6"}:         toggleThinking("deepseek", false),
	{Provider: "opencode-go", ModelID: "mimo-v2.5"}:         toggleThinking("deepseek", true),
	{Provider: "opencode-go", ModelID: "mimo-v2.5-pro"}:     fixedThinking("", true),
	{Provider: "opencode-go", ModelID: "minimax-m2.7"}:      fixedThinking("hidden", false),
	{Provider: "opencode-go", ModelID: "qwen3.6-plus"}:      toggleThinking("qwen", false),
}

func normalizeOpenCodeModel(candidate *model, source sourceModel) {
	applyMiniMaxThinkingMetadata(candidate, source.ReasoningOptions)
}

func fromOpenCode(catalog map[string]sourceProvider) []model {
	var models []model
	for _, variant := range openCodeVariants {
		for id, source := range catalog[variant.Source].Models {
			if !source.ToolCall || source.Status == "deprecated" {
				continue
			}
			protocol := "openai-completions"
			baseURL := variant.BaseURL + "/v1"
			compat := compatibility{Kind: "openai", MaxTokensField: "max_tokens"}
			switch source.Provider.NPM {
			case "@ai-sdk/anthropic":
				protocol = "anthropic-messages"
				baseURL = variant.BaseURL
				compat = compatibility{}
			case "@ai-sdk/openai":
				protocol = "openai-responses"
				compat = compatibility{}
			case "@ai-sdk/google":
				// Google Generative AI does not have a built-in adapter yet.
				continue
			}
			// These Anthropic-labeled models use the OpenAI-compatible path.
			if variant.Provider == "opencode-go" && (id == "minimax-m2.7" || id == "qwen3.5-plus" || id == "qwen3.6-plus") {
				protocol = "openai-completions"
				baseURL = variant.BaseURL + "/v1"
				compat = compatibility{Kind: "openai", MaxTokensField: "max_tokens"}
				if strings.HasPrefix(id, "qwen") {
					compat.ThinkingFormat = "qwen"
				}
			}
			if protocol != "openai-completions" && protocol != "openai-responses" && protocol != "anthropic-messages" {
				continue
			}
			models = append(models, normalize(id, source, providerRule{
				Provider: variant.Provider, Protocol: protocol, BaseURL: baseURL, Compat: compat,
				Normalize: normalizeOpenCodeModel,
			}))
		}
	}
	return models
}

func applyOpenCodeOverrides(candidate *model) {
	if candidate == nil {
		return
	}
	if candidate.Provider != "opencode" && candidate.Provider != "opencode-go" {
		return
	}
	if candidate.ID == "claude-sonnet-4" || candidate.ID == "claude-sonnet-4-5" {
		candidate.ContextWindow = 200_000
	}
	if candidate.Protocol != "openai-completions" {
		return
	}

	key := modelRouteKey{Provider: candidate.Provider, ModelID: candidate.ID}
	if profile, ok := openCodeThinkingProfiles[key]; ok {
		applyThinkingProfile(candidate, profile)
		return
	}

	// An empty models.dev control list does not prove that a gateway route can
	// disable or tune reasoning. Keep the provider default and expose one fixed
	// thinking state until that exact route has a verified profile.
	if candidate.Reasoning && candidate.ThinkingLevelMap == nil {
		applyThinkingProfile(candidate, fixedProviderDefault())
	}
}
