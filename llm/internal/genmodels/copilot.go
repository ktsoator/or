package main

import "strings"

const (
	githubCopilotSource   = "github-copilot"
	githubCopilotProvider = "github-copilot"
	githubCopilotBaseURL  = "https://api.individual.githubcopilot.com"
)

func fromCopilot(catalog map[string]sourceProvider) []model {
	var models []model
	for id, source := range catalog[githubCopilotSource].Models {
		if !source.ToolCall || source.Status == "deprecated" || strings.HasPrefix(id, "gpt-5") || strings.HasPrefix(id, "oswe") {
			continue
		}
		protocol := "openai-completions"
		compat := compatibility{
			Kind: "openai", SupportsStore: boolp(false), SupportsDeveloperRole: boolp(false), SupportsReasoningEffort: boolp(false),
		}
		if isCopilotClaude4(id) {
			protocol = "anthropic-messages"
			compat = compatibility{Kind: "anthropic"}
		}
		candidate := normalize(id, source, providerRule{
			Provider: githubCopilotProvider, Protocol: protocol, BaseURL: githubCopilotBaseURL,
			Compat: compat,
			Headers: map[string]string{
				"User-Agent": "GitHubCopilotChat/0.35.0", "Editor-Version": "vscode/1.107.0",
				"Editor-Plugin-Version": "copilot-chat/0.35.0", "Copilot-Integration-Id": "vscode-chat",
			},
		})
		if candidate.Reasoning && candidate.Protocol == "openai-completions" {
			applyThinkingProfile(&candidate, fixedProviderDefault())
		}
		models = append(models, candidate)
	}
	return models
}

func isCopilotClaude4(id string) bool {
	return strings.HasPrefix(id, "claude-haiku-4") || strings.HasPrefix(id, "claude-sonnet-4") || strings.HasPrefix(id, "claude-opus-4")
}
