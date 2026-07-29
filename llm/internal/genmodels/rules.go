package main

import (
	"fmt"
	"net/url"
)

var providerRules = []providerRule{
	{Source: "anthropic", Provider: "anthropic", Protocol: "anthropic-messages", BaseURL: "https://api.anthropic.com"},
	{Source: "deepseek", Provider: "deepseek", Protocol: "openai-completions", BaseURL: "https://api.deepseek.com", Normalize: normalizeDeepSeekModel},
	{Source: "groq", Provider: "groq", Protocol: "openai-completions", BaseURL: "https://api.groq.com/openai/v1"},
	{Source: "cerebras", Provider: "cerebras", Protocol: "openai-completions", BaseURL: "https://api.cerebras.ai/v1"},
	{Source: "xai", Provider: "xai", Protocol: "openai-completions", BaseURL: "https://api.x.ai/v1"},
	{Source: "nvidia", Provider: "nvidia", Protocol: "openai-completions", BaseURL: "https://integrate.api.nvidia.com/v1", Normalize: normalizeNVIDIAModel},
	{Source: "togetherai", Provider: "together", Protocol: "openai-completions", BaseURL: "https://api.together.ai/v1", Normalize: normalizeTogetherModel},
	{Source: "huggingface", Provider: "huggingface", Protocol: "openai-completions", BaseURL: "https://router.huggingface.co/v1", Compat: openAICompat(withDeveloperRole(false))},
	{Source: "fireworks-ai", Provider: "fireworks", Protocol: "anthropic-messages", BaseURL: "https://api.fireworks.ai/inference", Normalize: normalizeFireworksModel},
	{Source: "minimax", Provider: "minimax", Protocol: "anthropic-messages", BaseURL: "https://api.minimax.io/anthropic", Normalize: normalizeMiniMaxModel},
	{Source: "minimax-cn", Provider: "minimax-cn", Protocol: "anthropic-messages", BaseURL: "https://api.minimaxi.com/anthropic", Normalize: normalizeMiniMaxModel},
	{Source: "moonshotai", Provider: "moonshotai", Protocol: "openai-completions", BaseURL: "https://api.moonshot.ai/v1", Compat: moonshotCompat(), Normalize: normalizeMoonshotModel},
	{Source: "moonshotai-cn", Provider: "moonshotai-cn", Protocol: "openai-completions", BaseURL: "https://api.moonshot.cn/v1", Compat: moonshotCompat(), Normalize: normalizeMoonshotModel},
	{Source: "xiaomi", Provider: "xiaomi", Protocol: "openai-completions", BaseURL: "https://api.xiaomimimo.com/v1", Normalize: normalizeXiaomiModel},
	{Source: "xiaomi-token-plan-cn", Provider: "xiaomi-token-plan-cn", Protocol: "openai-completions", BaseURL: "https://token-plan-cn.xiaomimimo.com/v1", Normalize: normalizeXiaomiModel},
	{Source: "xiaomi-token-plan-ams", Provider: "xiaomi-token-plan-ams", Protocol: "openai-completions", BaseURL: "https://token-plan-ams.xiaomimimo.com/v1", Normalize: normalizeXiaomiModel},
	{Source: "xiaomi-token-plan-sgp", Provider: "xiaomi-token-plan-sgp", Protocol: "openai-completions", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1", Normalize: normalizeXiaomiModel},
	{Source: "zai-coding-plan", Provider: "zai", Protocol: "openai-completions", BaseURL: "https://api.z.ai/api/coding/paas/v4", Compat: zaiCompat(), Normalize: normalizeZAIModel},
	{Source: "zai-coding-plan", Provider: "zai-coding-cn", Protocol: "openai-completions", BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4", Compat: zaiCompat(), Normalize: normalizeZAIModel},
	{Source: "kimi-for-coding", Provider: "kimi-coding", Protocol: "anthropic-messages", BaseURL: "https://api.kimi.com/coding", Headers: map[string]string{"User-Agent": "KimiCLI/1.5"}, Normalize: normalizeKimiCodingModel},

	// Catalog-only providers are listed for discovery, but no adapter implements
	// their protocol yet. Keep them free of protocol compatibility overrides.
	{Source: "openai", Provider: "openai", Protocol: "openai-responses", BaseURL: "https://api.openai.com/v1"},
	{Source: "google", Provider: "google", Protocol: "google-generative-ai", BaseURL: "https://generativelanguage.googleapis.com/v1beta"},
	{Source: "mistral", Provider: "mistral", Protocol: "mistral-conversations", BaseURL: "https://api.mistral.ai"},
}

type compatOption func(*compatibility)

func openAICompat(options ...compatOption) compatibility {
	c := compatibility{Kind: "openai"}
	for _, option := range options {
		option(&c)
	}
	return c
}

func withDeveloperRole(value bool) compatOption {
	return func(c *compatibility) { c.SupportsDeveloperRole = boolp(value) }
}

func moonshotCompat() compatibility {
	return compatibility{
		Kind:                    "openai",
		SupportsStore:           boolp(false),
		SupportsDeveloperRole:   boolp(false),
		SupportsReasoningEffort: boolp(false),
		MaxTokensField:          "max_tokens",
		SupportsStrictMode:      boolp(false),
		ThinkingFormat:          "deepseek",
	}
}

func zaiCompat() compatibility {
	return compatibility{
		Kind:                    "openai",
		SupportsDeveloperRole:   boolp(false),
		SupportsReasoningEffort: boolp(false),
		ThinkingFormat:          "zai",
		ZAIToolStream:           boolp(true),
	}
}

func validateProviderRules(catalog map[string]sourceProvider) error {
	return validateProviderRuleSet(providerRules, catalog)
}

func validateProviderRuleSet(rules []providerRule, catalog map[string]sourceProvider) error {
	seenProviders := make(map[string]int, len(rules))
	for index, rule := range rules {
		if rule.Source == "" {
			return fmt.Errorf("provider rule %d has an empty source", index)
		}
		if rule.Provider == "" {
			return fmt.Errorf("provider rule %d has an empty provider", index)
		}
		if rule.Protocol == "" {
			return fmt.Errorf("provider %q has an empty protocol", rule.Provider)
		}
		parsedBaseURL, err := url.Parse(rule.BaseURL)
		if err != nil || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") || parsedBaseURL.Host == "" {
			return fmt.Errorf("provider %q has an invalid base URL %q", rule.Provider, rule.BaseURL)
		}
		if previous, exists := seenProviders[rule.Provider]; exists {
			return fmt.Errorf("provider rule %d duplicates provider %q from rule %d", index, rule.Provider, previous)
		}
		seenProviders[rule.Provider] = index

		source, exists := catalog[rule.Source]
		if !exists {
			return fmt.Errorf("provider %q references missing models.dev source %q", rule.Provider, rule.Source)
		}
		if !hasUsableSourceModel(source) {
			return fmt.Errorf("provider %q source %q has no usable tool-calling models", rule.Provider, rule.Source)
		}
	}
	return nil
}

func hasUsableSourceModel(source sourceProvider) bool {
	for _, candidate := range source.Models {
		if candidate.ToolCall && candidate.Status != "deprecated" {
			return true
		}
	}
	return false
}

func fromModelsDev(catalog map[string]sourceProvider) []model {
	var models []model
	for _, rule := range providerRules {
		for id, source := range catalog[rule.Source].Models {
			if !source.ToolCall || source.Status == "deprecated" {
				continue
			}
			models = append(models, normalize(id, source, rule))
		}
	}
	models = append(models, fromOpenCode(catalog)...)
	models = append(models, fromCopilot(catalog)...)
	return models
}

func normalize(id string, source sourceModel, rule providerRule) model {
	name := source.Name
	if name == "" {
		name = id
	}
	candidate := model{
		ID:             id,
		Name:           name,
		Protocol:       rule.Protocol,
		Provider:       rule.Provider,
		BaseURL:        rule.BaseURL,
		Reasoning:      source.Reasoning,
		Input:          inputModalities(source.Modalities.Input),
		InputCost:      source.Cost.Input,
		OutputCost:     source.Cost.Output,
		CacheReadCost:  source.Cost.CacheRead,
		CacheWriteCost: source.Cost.CacheWrite,
		ContextWindow:  defaultInt(source.Limit.Context, 4096),
		MaxTokens:      defaultInt(source.Limit.Output, 4096),
		Headers:        cloneMap(rule.Headers),
		Compat:         rule.Compat,
	}
	if rule.Normalize != nil {
		rule.Normalize(&candidate, source)
	}
	applyReasoningOptionMetadata(&candidate, source.ReasoningOptions)
	return candidate
}
