package main

type sourceModel struct {
	Name             string                  `json:"name"`
	ToolCall         bool                    `json:"tool_call"`
	Reasoning        bool                    `json:"reasoning"`
	ReasoningOptions []sourceReasoningOption `json:"reasoning_options"`
	Status           string                  `json:"status"`
	Limit            struct {
		Context int64 `json:"context"`
		Output  int64 `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	} `json:"cost"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Provider struct {
		NPM string `json:"npm"`
	} `json:"provider"`
}

type sourceReasoningOption struct {
	Type   string    `json:"type"`
	Values []*string `json:"values"`
}

type sourceProvider struct {
	Models map[string]sourceModel `json:"models"`
}

type model struct {
	ID                 string
	Name               string
	Protocol           string
	Provider           string
	BaseURL            string
	Reasoning          bool
	Input              []string
	InputCost          float64
	OutputCost         float64
	CacheReadCost      float64
	CacheWriteCost     float64
	ContextWindow      int64
	MaxTokens          int64
	Headers            map[string]string
	ThinkingLevelMap   map[string]*string
	ThinkingVisibility string
	Compat             compatibility
}

type compatibility struct {
	Kind                                        string `json:"-"`
	SupportsStore                               *bool  `json:"supportsStore,omitempty"`
	SupportsDeveloperRole                       *bool  `json:"supportsDeveloperRole,omitempty"`
	SupportsReasoningEffort                     *bool  `json:"supportsReasoningEffort,omitempty"`
	MaxTokensField                              string `json:"maxTokensField,omitempty"`
	SupportsStrictMode                          *bool  `json:"supportsStrictMode,omitempty"`
	RequiresReasoningContentOnAssistantMessages *bool  `json:"requiresReasoningContentOnAssistantMessages,omitempty"`
	RequiresThinkingAsText                      *bool  `json:"requiresThinkingAsText,omitempty"`
	ThinkingFormat                              string `json:"thinkingFormat,omitempty"`
	ZAIToolStream                               *bool  `json:"zaiToolStream,omitempty"`
	SupportsTemperature                         *bool  `json:"supportsTemperature,omitempty"`
	SupportsCacheControl                        *bool  `json:"supportsCacheControl,omitempty"`
	SupportsCacheControlTools                   *bool  `json:"supportsCacheControlOnTools,omitempty"`
	ForceAdaptiveThinking                       *bool  `json:"forceAdaptiveThinking,omitempty"`
	AllowEmptySignature                         *bool  `json:"allowEmptySignature,omitempty"`
}

type catalogModel struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Provider           string             `json:"provider"`
	Protocol           string             `json:"protocol"`
	BaseURL            string             `json:"baseUrl"`
	Reasoning          bool               `json:"reasoning"`
	ThinkingLevelMap   map[string]*string `json:"thinkingLevelMap,omitempty"`
	ThinkingVisibility string             `json:"thinkingVisibility,omitempty"`
	Input              []string           `json:"input"`
	Cost               catalogCost        `json:"cost"`
	ContextWindow      int64              `json:"contextWindow"`
	MaxTokens          int64              `json:"maxTokens"`
	Headers            map[string]string  `json:"headers,omitempty"`
	Compatibility      *compatibility     `json:"compat,omitempty"`
}

type catalogCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// modelNormalizer compiles source metadata for one routed provider. It must not
// infer behavior for other endpoints that happen to serve the same model family.
type modelNormalizer func(*model, sourceModel)

type providerRule struct {
	Source    string
	Provider  string
	Protocol  string
	BaseURL   string
	Compat    compatibility
	Headers   map[string]string
	Normalize modelNormalizer
}

func boolp(value bool) *bool { return &value }
