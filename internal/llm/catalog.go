package llm

var builtInModelRegistry = newBuiltInModelRegistry()

func newBuiltInModelRegistry() *ModelRegistry {
	registry := NewModelRegistry()
	for _, model := range builtInModels() {
		if err := registry.Register(model); err != nil {
			panic(err)
		}
	}
	return registry
}

func builtInModels() []Model {
	return []Model{
		{
			ID:            "deepseek-v4-flash",
			Name:          "DeepSeek V4 Flash",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "deepseek",
			BaseURL:       "https://api.deepseek.com",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 0.14, Output: 0.28, CacheRead: 0.0028},
			ContextWindow: 1_000_000,
			MaxTokens:     384_000,
			ThinkingLevelMap: map[ModelThinkingLevel]*string{
				ModelThinkingMinimal: nil,
				ModelThinkingLow:     nil,
				ModelThinkingMedium:  nil,
				ModelThinkingHigh:    stringPointer("high"),
				ModelThinkingXHigh:   stringPointer("max"),
			},
		},
		{
			ID:            "deepseek-v4-pro",
			Name:          "DeepSeek V4 Pro",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "deepseek",
			BaseURL:       "https://api.deepseek.com",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 0.435, Output: 0.87, CacheRead: 0.003625},
			ContextWindow: 1_000_000,
			MaxTokens:     384_000,
			ThinkingLevelMap: map[ModelThinkingLevel]*string{
				ModelThinkingMinimal: nil,
				ModelThinkingLow:     nil,
				ModelThinkingMedium:  nil,
				ModelThinkingHigh:    stringPointer("high"),
				ModelThinkingXHigh:   stringPointer("max"),
			},
		},
		{
			ID:            "kimi-k2.5",
			Name:          "Kimi K2.5",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "moonshotai",
			BaseURL:       "https://api.moonshot.ai/v1",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 0.6, Output: 3, CacheRead: 0.1},
			ContextWindow: 262_144,
			MaxTokens:     262_144,
			Compatibility: moonshotCompatibility(),
		},
		{
			ID:            "kimi-k2.6",
			Name:          "Kimi K2.6",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "moonshotai",
			BaseURL:       "https://api.moonshot.ai/v1",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 0.95, Output: 4, CacheRead: 0.16},
			ContextWindow: 262_144,
			MaxTokens:     262_144,
			Compatibility: moonshotCompatibility(),
		},
		{
			ID:            "kimi-k2.6",
			Name:          "Kimi K2.6",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "moonshotai-cn",
			BaseURL:       "https://api.moonshot.cn/v1",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 0.95, Output: 4, CacheRead: 0.16},
			ContextWindow: 262_144,
			MaxTokens:     262_144,
			Compatibility: moonshotCompatibility(),
		},
		{
			ID:            "mimo-v2.5-pro",
			Name:          "MiMo-V2.5-Pro",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "xiaomi",
			BaseURL:       "https://api.xiaomimimo.com/v1",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 1, Output: 3, CacheRead: 0.2},
			ContextWindow: 1_048_576,
			MaxTokens:     131_072,
			Compatibility: &OpenAICompletionsCompatibility{
				RequiresReasoningContentOnAssistantMessages: boolPointer(true),
				ThinkingFormat: "deepseek",
			},
		},
		{
			ID:            "glm-4.7",
			Name:          "GLM-4.7",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai",
			BaseURL:       "https://api.z.ai/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 204_800,
			MaxTokens:     131_072,
			Compatibility: zaiCompatibility(nil),
		},
		{
			ID:            "glm-5-turbo",
			Name:          "GLM-5-Turbo",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai",
			BaseURL:       "https://api.z.ai/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 200_000,
			MaxTokens:     131_072,
			Compatibility: zaiCompatibility(nil),
		},
		{
			ID:            "glm-5.1",
			Name:          "GLM-5.1",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai",
			BaseURL:       "https://api.z.ai/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 200_000,
			MaxTokens:     131_072,
			Compatibility: zaiCompatibility(nil),
		},
		{
			ID:            "glm-5.2",
			Name:          "GLM-5.2",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai",
			BaseURL:       "https://api.z.ai/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 1_000_000,
			MaxTokens:     131_072,
			ThinkingLevelMap: map[ModelThinkingLevel]*string{
				ModelThinkingMinimal: nil,
				ModelThinkingLow:     stringPointer("high"),
				ModelThinkingMedium:  stringPointer("high"),
				ModelThinkingHigh:    stringPointer("high"),
				ModelThinkingXHigh:   stringPointer("max"),
			},
			Compatibility: zaiCompatibility(boolPointer(true)),
		},
		{
			ID:            "glm-5-turbo",
			Name:          "GLM-5-Turbo",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai-coding-cn",
			BaseURL:       "https://open.bigmodel.cn/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 200_000,
			MaxTokens:     131_072,
			Compatibility: zaiCompatibility(nil),
		},
		{
			ID:            "glm-5.1",
			Name:          "GLM-5.1",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai-coding-cn",
			BaseURL:       "https://open.bigmodel.cn/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 200_000,
			MaxTokens:     131_072,
			Compatibility: zaiCompatibility(nil),
		},
		{
			ID:            "glm-5.2",
			Name:          "GLM-5.2",
			Protocol:      ProtocolOpenAICompletions,
			Provider:      "zai-coding-cn",
			BaseURL:       "https://open.bigmodel.cn/api/coding/paas/v4",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			ContextWindow: 1_000_000,
			MaxTokens:     131_072,
			ThinkingLevelMap: map[ModelThinkingLevel]*string{
				ModelThinkingMinimal: nil,
				ModelThinkingLow:     stringPointer("high"),
				ModelThinkingMedium:  stringPointer("high"),
				ModelThinkingHigh:    stringPointer("high"),
				ModelThinkingXHigh:   stringPointer("max"),
			},
			Compatibility: zaiCompatibility(boolPointer(true)),
		},
		{
			ID:            "MiniMax-M2.7",
			Name:          "MiniMax-M2.7",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "minimax",
			BaseURL:       "https://api.minimax.io/anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 0.3, Output: 1.2, CacheRead: 0.06, CacheWrite: 0.375},
			ContextWindow: 204_800,
			MaxTokens:     131_072,
		},
		{
			ID:            "MiniMax-M2.7-highspeed",
			Name:          "MiniMax-M2.7-highspeed",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "minimax",
			BaseURL:       "https://api.minimax.io/anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 0.6, Output: 2.4, CacheRead: 0.06, CacheWrite: 0.375},
			ContextWindow: 204_800,
			MaxTokens:     131_072,
		},
		{
			ID:            "MiniMax-M3",
			Name:          "MiniMax-M3",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "minimax",
			BaseURL:       "https://api.minimax.io/anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 0.6, Output: 2.4, CacheRead: 0.12, CacheWrite: 0},
			ContextWindow: 512_000,
			MaxTokens:     128_000,
		},
		{
			ID:            "MiniMax-M2.7",
			Name:          "MiniMax-M2.7",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "minimax-cn",
			BaseURL:       "https://api.minimaxi.com/anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 0.3, Output: 1.2, CacheRead: 0.06, CacheWrite: 0.375},
			ContextWindow: 204_800,
			MaxTokens:     131_072,
		},
		{
			ID:            "MiniMax-M2.7-highspeed",
			Name:          "MiniMax-M2.7-highspeed",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "minimax-cn",
			BaseURL:       "https://api.minimaxi.com/anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText},
			Cost:          ModelCost{Input: 0.6, Output: 2.4, CacheRead: 0.06, CacheWrite: 0.375},
			ContextWindow: 204_800,
			MaxTokens:     131_072,
		},
		{
			ID:            "MiniMax-M3",
			Name:          "MiniMax-M3",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "minimax-cn",
			BaseURL:       "https://api.minimaxi.com/anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 0.6, Output: 2.4, CacheRead: 0.12, CacheWrite: 0},
			ContextWindow: 512_000,
			MaxTokens:     128_000,
		},
		{
			ID:            "claude-opus-4-8",
			Name:          "Claude Opus 4.8",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 5, Output: 25, CacheRead: 0.5, CacheWrite: 6.25},
			ContextWindow: 1_000_000,
			MaxTokens:     64_000,
			// Opus 4.7+ uses adaptive thinking and rejects the temperature field.
			Compatibility: &AnthropicMessagesCompatibility{
				ForceAdaptiveThinking: boolPointer(true),
				SupportsTemperature:   boolPointer(false),
			},
		},
		{
			ID:            "claude-sonnet-4-6",
			Name:          "Claude Sonnet 4.6",
			Protocol:      ProtocolAnthropicMessages,
			Provider:      "anthropic",
			Reasoning:     true,
			Input:         []ModelInput{ModelInputText, ModelInputImage},
			Cost:          ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75},
			ContextWindow: 1_000_000,
			MaxTokens:     64_000,
			Compatibility: &AnthropicMessagesCompatibility{
				ForceAdaptiveThinking: boolPointer(true),
			},
		},
	}
}

func moonshotCompatibility() *OpenAICompletionsCompatibility {
	// Detection already covers store, developer role, reasoning effort, the
	// max-tokens field, and strict mode for Moonshot; only the reasoning wire
	// format (Moonshot uses the DeepSeek shape) needs an explicit override.
	return &OpenAICompletionsCompatibility{
		ThinkingFormat: "deepseek",
	}
}

func zaiCompatibility(supportsReasoningEffort *bool) *OpenAICompletionsCompatibility {
	// Detection covers the developer role and the zai thinking format; tool
	// streaming and the per-model reasoning-effort opt-in are not detectable.
	return &OpenAICompletionsCompatibility{
		SupportsReasoningEffort: supportsReasoningEffort,
		ZAIToolStream:           boolPointer(true),
	}
}

func stringPointer(value string) *string {
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}
