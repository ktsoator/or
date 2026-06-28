package openai

import (
	"strings"

	"github.com/ktsoator/or/llm"
)

// resolvedCompat holds the OpenAI-compatible quirks for a model with every value
// concrete. It is the result of auto-detection from provider/baseURL overlaid
// with the model's explicit compatibility overrides. Only the fields the adapter
// actually consumes are resolved here.
type resolvedCompat struct {
	supportsStore                               bool
	supportsDeveloperRole                       bool
	supportsReasoningEffort                     bool
	maxTokensField                              string
	requiresReasoningContentOnAssistantMessages bool
	requiresThinkingAsText                      bool
	thinkingFormat                              string
	zaiToolStream                               bool
	supportsStrictMode                          bool
}

// resolveCompat returns the model's compatibility settings: auto-detected from
// provider/baseURL, then overridden by any explicit fields on model.Compatibility.
func resolveCompat(model llm.Model) resolvedCompat {
	compat := detectCompat(model)

	override, ok := model.Compatibility.(*llm.OpenAICompletionsCompatibility)
	if !ok || override == nil {
		return compat
	}
	if override.SupportsStore != nil {
		compat.supportsStore = *override.SupportsStore
	}
	if override.SupportsDeveloperRole != nil {
		compat.supportsDeveloperRole = *override.SupportsDeveloperRole
	}
	if override.SupportsReasoningEffort != nil {
		compat.supportsReasoningEffort = *override.SupportsReasoningEffort
	}
	if override.MaxTokensField != "" {
		compat.maxTokensField = override.MaxTokensField
	}
	if override.RequiresReasoningContentOnAssistantMessages != nil {
		compat.requiresReasoningContentOnAssistantMessages = *override.RequiresReasoningContentOnAssistantMessages
	}
	if override.RequiresThinkingAsText != nil {
		compat.requiresThinkingAsText = *override.RequiresThinkingAsText
	}
	if override.ThinkingFormat != "" {
		compat.thinkingFormat = override.ThinkingFormat
	}
	if override.ZAIToolStream != nil {
		compat.zaiToolStream = *override.ZAIToolStream
	}
	if override.SupportsStrictMode != nil {
		compat.supportsStrictMode = *override.SupportsStrictMode
	}
	return compat
}

// detectCompat infers compatibility settings from the model's provider and
// baseURL for known OpenAI-compatible endpoints. It mirrors the reference
// detection table so most models need no explicit Compatibility override.
func detectCompat(model llm.Model) resolvedCompat {
	provider := model.Provider
	contains := func(needle string) bool { return strings.Contains(model.BaseURL, needle) }

	isZai := provider == "zai" || provider == "zai-coding-cn" ||
		contains("api.z.ai") || contains("open.bigmodel.cn")
	isTogether := provider == "together" || contains("api.together.ai") || contains("api.together.xyz")
	isMoonshot := provider == "moonshotai" || provider == "moonshotai-cn" || contains("api.moonshot.")
	isOpenRouter := provider == "openrouter" || contains("openrouter.ai")
	isCloudflareWorkersAI := provider == "cloudflare-workers-ai" || contains("api.cloudflare.com")
	isCloudflareAiGateway := provider == "cloudflare-ai-gateway" || contains("gateway.ai.cloudflare.com")
	isNvidia := provider == "nvidia" || contains("integrate.api.nvidia.com")
	isAntLing := provider == "ant-ling" || contains("api.ant-ling.com")

	isNonStandard := isNvidia ||
		provider == "cerebras" || contains("cerebras.ai") ||
		provider == "xai" || contains("api.x.ai") ||
		isTogether || contains("chutes.ai") || contains("deepseek.com") ||
		isZai || isMoonshot ||
		provider == "opencode" || contains("opencode.ai") ||
		isCloudflareWorkersAI || isCloudflareAiGateway || isAntLing

	useMaxTokens := contains("chutes.ai") || isMoonshot || isCloudflareAiGateway ||
		isTogether || isNvidia || isAntLing

	isGrok := provider == "xai" || contains("api.x.ai")
	isDeepSeek := provider == "deepseek" || contains("deepseek.com")
	isOpenRouterDeveloperRoleModel := isOpenRouter &&
		(strings.HasPrefix(model.ID, "anthropic/") || strings.HasPrefix(model.ID, "openai/"))

	maxTokensField := "max_completion_tokens"
	if useMaxTokens {
		maxTokensField = "max_tokens"
	}

	thinkingFormat := "openai"
	switch {
	case isDeepSeek:
		thinkingFormat = "deepseek"
	case isZai:
		thinkingFormat = "zai"
	case isTogether:
		thinkingFormat = "together"
	case isAntLing:
		thinkingFormat = "ant-ling"
	case isOpenRouter:
		thinkingFormat = "openrouter"
	}

	return resolvedCompat{
		supportsStore:         !isNonStandard,
		supportsDeveloperRole: isOpenRouterDeveloperRoleModel || (!isNonStandard && !isOpenRouter),
		supportsReasoningEffort: !isGrok && !isZai && !isMoonshot && !isTogether &&
			!isCloudflareAiGateway && !isNvidia && !isAntLing,
		maxTokensField: maxTokensField,
		requiresReasoningContentOnAssistantMessages: isDeepSeek,
		thinkingFormat:     thinkingFormat,
		zaiToolStream:      false,
		supportsStrictMode: !isMoonshot && !isTogether && !isCloudflareAiGateway && !isNvidia,
	}
}
