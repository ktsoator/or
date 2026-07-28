package main

import "strings"

func fromOpenCode(catalog map[string]sourceProvider) []model {
	variants := []struct{ source, provider, base string }{
		{"opencode", "opencode", "https://opencode.ai/zen"},
		{"opencode-go", "opencode-go", "https://opencode.ai/zen/go"},
	}
	var models []model
	for _, variant := range variants {
		for id, source := range catalog[variant.source].Models {
			if !source.ToolCall || source.Status == "deprecated" {
				continue
			}
			protocol := "openai-completions"
			baseURL := variant.base + "/v1"
			compat := compatibility{Kind: "openai", MaxTokensField: "max_tokens"}
			switch source.Provider.NPM {
			case "@ai-sdk/anthropic":
				protocol = "anthropic-messages"
				baseURL = variant.base
				compat = compatibility{}
			case "@ai-sdk/openai", "@ai-sdk/google":
				// These require protocols that the Go package does not implement yet.
				continue
			}
			// These models are mislabeled upstream and use the OpenAI-compatible path.
			if variant.provider == "opencode-go" && (id == "minimax-m2.7" || id == "qwen3.5-plus" || id == "qwen3.6-plus") {
				protocol = "openai-completions"
				baseURL = variant.base + "/v1"
				compat = compatibility{Kind: "openai", MaxTokensField: "max_tokens"}
				if strings.HasPrefix(id, "qwen") {
					compat.ThinkingFormat = "qwen"
				}
			}
			if protocol != "openai-completions" && protocol != "anthropic-messages" {
				continue
			}
			normalized := normalize(id, source, providerRule{
				Provider: variant.provider, Protocol: protocol, BaseURL: baseURL, Compat: compat,
			})
			applyOpenCodeOverrides(&normalized)
			models = append(models, normalized)
		}
	}
	return models
}

func applyOpenCodeOverrides(candidate *model) {
	if candidate == nil {
		return
	}
	if (candidate.Provider == "opencode" || candidate.Provider == "opencode-go") &&
		(candidate.ID == "claude-sonnet-4" || candidate.ID == "claude-sonnet-4-5") {
		candidate.ContextWindow = 200_000
	}
	if candidate.Protocol != "openai-completions" {
		return
	}

	if candidate.ID == "kimi-k2.6" {
		candidate.Compat.ThinkingFormat = "deepseek"
		candidate.Compat.SupportsReasoningEffort = boolp(false)
		candidate.ThinkingLevelMap = map[string]*string{
			"minimal": nil,
			"low":     nil,
			"medium":  nil,
		}
	}

	if candidate.Provider == "opencode-go" && candidate.ID == "minimax-m2.7" {
		candidate.Compat.SupportsReasoningEffort = boolp(false)
		candidate.ThinkingLevelMap = map[string]*string{
			"off":     nil,
			"minimal": nil,
			"low":     nil,
			"medium":  nil,
		}
	}

	if strings.Contains(candidate.ID, "deepseek-v4") {
		candidate.Compat.RequiresReasoningContentOnAssistantMessages = boolp(true)
		if candidate.Provider == "opencode-go" {
			candidate.Compat.ThinkingFormat = "deepseek"
		}
	}

	if candidate.Provider == "opencode-go" && candidate.ID == "glm-5.2" {
		high, max := "high", "max"
		candidate.ThinkingLevelMap = map[string]*string{
			"off":     nil,
			"minimal": nil,
			"low":     nil,
			"medium":  nil,
			"high":    &high,
			"max":     &max,
		}
	}

	if candidate.Provider == "opencode" && candidate.ID == "grok-build-0.1" {
		candidate.Compat.SupportsReasoningEffort = boolp(false)
		candidate.ThinkingLevelMap = map[string]*string{
			"off":     nil,
			"minimal": nil,
			"low":     nil,
			"medium":  nil,
		}
	}
}
