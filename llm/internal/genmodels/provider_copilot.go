package main

import "strings"

func fromCopilot(catalog map[string]sourceProvider) []model {
	var models []model
	for id, source := range catalog["github-copilot"].Models {
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
		models = append(models, normalize(id, source, providerRule{
			Provider: "github-copilot", Protocol: protocol, BaseURL: "https://api.individual.githubcopilot.com",
			Compat: compat,
			Headers: map[string]string{
				"User-Agent": "GitHubCopilotChat/0.35.0", "Editor-Version": "vscode/1.107.0",
				"Editor-Plugin-Version": "copilot-chat/0.35.0", "Copilot-Integration-Id": "vscode-chat",
			},
		}))
	}
	return models
}

func isCopilotClaude4(id string) bool {
	return strings.HasPrefix(id, "claude-haiku-4") || strings.HasPrefix(id, "claude-sonnet-4") || strings.HasPrefix(id, "claude-opus-4")
}
