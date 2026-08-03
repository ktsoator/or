package chatcompletions

import (
	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/internal/openaicompat"
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
	return fromResolvedCompat(openaicompat.Resolve(model))
}

// detectCompat infers compatibility settings from the model's provider and
// baseURL for known OpenAI-compatible endpoints. It mirrors the reference
// detection table so most models need no explicit Compatibility override.
func detectCompat(model llm.Model) resolvedCompat {
	return fromResolvedCompat(openaicompat.Detect(model))
}

func fromResolvedCompat(compat openaicompat.Resolved) resolvedCompat {
	return resolvedCompat{
		supportsStore:                               compat.SupportsStore,
		supportsDeveloperRole:                       compat.SupportsDeveloperRole,
		supportsReasoningEffort:                     compat.SupportsReasoningEffort,
		maxTokensField:                              compat.MaxTokensField,
		requiresReasoningContentOnAssistantMessages: compat.RequiresReasoningContentOnAssistantMessages,
		requiresThinkingAsText:                      compat.RequiresThinkingAsText,
		thinkingFormat:                              string(compat.ThinkingFormat),
		zaiToolStream:                               compat.ZAIToolStream,
		supportsStrictMode:                          compat.SupportsStrictMode,
	}
}
