// Package openaicompat resolves the effective wire behavior of models served
// through OpenAI-compatible Chat Completions endpoints.
package openaicompat

import (
	"strings"

	"github.com/ktsoator/or/llm"
)

// ThinkingFormat identifies the request shape used to control reasoning.
type ThinkingFormat string

const (
	ThinkingFormatOpenAI           ThinkingFormat = "openai"
	ThinkingFormatOpenRouter       ThinkingFormat = "openrouter"
	ThinkingFormatDeepSeek         ThinkingFormat = "deepseek"
	ThinkingFormatTogether         ThinkingFormat = "together"
	ThinkingFormatZAI              ThinkingFormat = "zai"
	ThinkingFormatQwen             ThinkingFormat = "qwen"
	ThinkingFormatQwenChatTemplate ThinkingFormat = "qwen-chat-template"
	ThinkingFormatString           ThinkingFormat = "string-thinking"
	ThinkingFormatAntLing          ThinkingFormat = "ant-ling"
)

// Resolved contains concrete compatibility values after provider and endpoint
// defaults have been overlaid with explicit model overrides.
type Resolved struct {
	SupportsStore                               bool
	SupportsDeveloperRole                       bool
	SupportsReasoningEffort                     bool
	MaxTokensField                              string
	RequiresReasoningContentOnAssistantMessages bool
	RequiresThinkingAsText                      bool
	ThinkingFormat                              ThinkingFormat
	ZAIToolStream                               bool
	SupportsStrictMode                          bool
}

// Resolve returns the effective compatibility settings for model.
func Resolve(model llm.Model) Resolved {
	compat := Detect(model)

	override, ok := model.Compatibility.(*llm.OpenAICompletionsCompatibility)
	if !ok || override == nil {
		return compat
	}
	if override.SupportsStore != nil {
		compat.SupportsStore = *override.SupportsStore
	}
	if override.SupportsDeveloperRole != nil {
		compat.SupportsDeveloperRole = *override.SupportsDeveloperRole
	}
	if override.SupportsReasoningEffort != nil {
		compat.SupportsReasoningEffort = *override.SupportsReasoningEffort
	}
	if override.MaxTokensField != "" {
		compat.MaxTokensField = override.MaxTokensField
	}
	if override.RequiresReasoningContentOnAssistantMessages != nil {
		compat.RequiresReasoningContentOnAssistantMessages = *override.RequiresReasoningContentOnAssistantMessages
	}
	if override.RequiresThinkingAsText != nil {
		compat.RequiresThinkingAsText = *override.RequiresThinkingAsText
	}
	if override.ThinkingFormat != "" {
		compat.ThinkingFormat = ThinkingFormat(override.ThinkingFormat)
	}
	if override.ZAIToolStream != nil {
		compat.ZAIToolStream = *override.ZAIToolStream
	}
	if override.SupportsStrictMode != nil {
		compat.SupportsStrictMode = *override.SupportsStrictMode
	}
	return compat
}

// Detect infers compatibility from the provider and endpoint. Explicit model
// compatibility must be applied through Resolve.
func Detect(model llm.Model) Resolved {
	provider := model.Provider
	contains := func(needle string) bool { return strings.Contains(model.BaseURL, needle) }

	isZAI := provider == "zai" || provider == "zai-coding-cn" ||
		contains("api.z.ai") || contains("open.bigmodel.cn")
	isTogether := provider == "together" || contains("api.together.ai") || contains("api.together.xyz")
	isMoonshot := provider == "moonshotai" || provider == "moonshotai-cn" || contains("api.moonshot.")
	isOpenRouter := provider == "openrouter" || contains("openrouter.ai")
	isCloudflareWorkersAI := provider == "cloudflare-workers-ai" || contains("api.cloudflare.com")
	isCloudflareAIGateway := provider == "cloudflare-ai-gateway" || contains("gateway.ai.cloudflare.com")
	isNVIDIA := provider == "nvidia" || contains("integrate.api.nvidia.com")
	isAntLing := provider == "ant-ling" || contains("api.ant-ling.com")

	isNonStandard := isNVIDIA ||
		provider == "cerebras" || contains("cerebras.ai") ||
		provider == "xai" || contains("api.x.ai") ||
		isTogether || contains("chutes.ai") || contains("deepseek.com") ||
		isZAI || isMoonshot ||
		provider == "opencode" || contains("opencode.ai") ||
		isCloudflareWorkersAI || isCloudflareAIGateway || isAntLing

	useMaxTokens := contains("chutes.ai") || isMoonshot || isCloudflareAIGateway ||
		isTogether || isNVIDIA || isAntLing

	isGrok := provider == "xai" || contains("api.x.ai")
	isDeepSeek := provider == "deepseek" || contains("deepseek.com")
	isOpenRouterDeveloperRoleModel := isOpenRouter &&
		(strings.HasPrefix(model.ID, "anthropic/") || strings.HasPrefix(model.ID, "openai/"))

	maxTokensField := "max_completion_tokens"
	if useMaxTokens {
		maxTokensField = "max_tokens"
	}

	thinkingFormat := ThinkingFormatOpenAI
	switch {
	case isDeepSeek:
		thinkingFormat = ThinkingFormatDeepSeek
	case isZAI:
		thinkingFormat = ThinkingFormatZAI
	case isTogether:
		thinkingFormat = ThinkingFormatTogether
	case isAntLing:
		thinkingFormat = ThinkingFormatAntLing
	case isOpenRouter:
		thinkingFormat = ThinkingFormatOpenRouter
	}

	return Resolved{
		SupportsStore:         !isNonStandard,
		SupportsDeveloperRole: isOpenRouterDeveloperRoleModel || (!isNonStandard && !isOpenRouter),
		SupportsReasoningEffort: !isGrok && !isZAI && !isMoonshot && !isTogether &&
			!isCloudflareAIGateway && !isNVIDIA && !isAntLing,
		MaxTokensField: maxTokensField,
		RequiresReasoningContentOnAssistantMessages: isDeepSeek,
		ThinkingFormat:     thinkingFormat,
		SupportsStrictMode: !isMoonshot && !isTogether && !isCloudflareAIGateway && !isNVIDIA,
	}
}

// SupportsDirectReasoningEffort reports whether models.dev effort values map
// directly to the top-level OpenAI reasoning_effort request field.
func SupportsDirectReasoningEffort(model llm.Model) bool {
	if model.Protocol != llm.ProtocolOpenAICompletions {
		return false
	}
	compat := Resolve(model)
	return compat.ThinkingFormat == ThinkingFormatOpenAI && compat.SupportsReasoningEffort
}
