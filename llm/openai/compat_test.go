package openai

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func boolPtr(b bool) *bool { return &b }

func TestDetectCompatOpenAIDefault(t *testing.T) {
	// A model that matches no known provider/baseURL keeps the OpenAI defaults:
	// store is on, developer role is allowed, thinking is the OpenAI dialect,
	// reasoning_effort is honored, and the max-tokens field is the new one.
	got := detectCompat(llm.Model{Provider: "openai", ID: "gpt-x"})

	if !got.supportsStore {
		t.Fatalf("supportsStore = false, want true for default")
	}
	if !got.supportsDeveloperRole {
		t.Fatalf("supportsDeveloperRole = false, want true for default")
	}
	if !got.supportsReasoningEffort {
		t.Fatalf("supportsReasoningEffort = false, want true for default")
	}
	if got.maxTokensField != "max_completion_tokens" {
		t.Fatalf("maxTokensField = %q, want max_completion_tokens", got.maxTokensField)
	}
	if got.thinkingFormat != "openai" {
		t.Fatalf("thinkingFormat = %q, want openai", got.thinkingFormat)
	}
	if !got.supportsStrictMode {
		t.Fatalf("supportsStrictMode = false, want true for default")
	}
}

func TestDetectCompatProviderMatrix(t *testing.T) {
	type want struct {
		thinkingFormat          string
		maxTokensField          string
		supportsReasoningEffort bool
		supportsStore           bool
		supportsDeveloperRole   bool
		supportsStrictMode      bool
		requiresReasoningOnAsst bool
	}
	defaults := want{
		thinkingFormat: "openai", maxTokensField: "max_completion_tokens",
		supportsReasoningEffort: true, supportsStore: true,
		supportsDeveloperRole: true, supportsStrictMode: true,
	}
	override := func(base want, mutate func(*want)) want {
		mutate(&base)
		return base
	}

	tests := []struct {
		name  string
		model llm.Model
		want  want
	}{
		{
			// Catalog deepseek entries always carry the deepseek.com baseURL,
			// which is what trips the non-standard checks. Provider name alone
			// is enough to pick the thinking dialect.
			name:  "deepseek with baseURL",
			model: llm.Model{Provider: "deepseek", ID: "deepseek-r1", BaseURL: "https://api.deepseek.com"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "deepseek"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.requiresReasoningOnAsst = true
			}),
		},
		{
			name:  "zai by provider",
			model: llm.Model{Provider: "zai", ID: "glm-4"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "zai"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
			}),
		},
		{
			name:  "zai-coding-cn by provider",
			model: llm.Model{Provider: "zai-coding-cn", ID: "glm-4-cn"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "zai"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
			}),
		},
		{
			name:  "moonshot uses max_tokens",
			model: llm.Model{Provider: "moonshotai", ID: "kimi"},
			want: override(defaults, func(w *want) {
				w.maxTokensField = "max_tokens"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
				w.supportsStrictMode = false
			}),
		},
		{
			name:  "together",
			model: llm.Model{Provider: "together", ID: "llama"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "together"
				w.maxTokensField = "max_tokens"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
				w.supportsStrictMode = false
			}),
		},
		{
			name:  "xai/grok disables reasoning_effort but keeps openai format",
			model: llm.Model{Provider: "xai", ID: "grok-2"},
			want: override(defaults, func(w *want) {
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
			}),
		},
		{
			name:  "nvidia",
			model: llm.Model{Provider: "nvidia", ID: "llama-nvidia"},
			want: override(defaults, func(w *want) {
				w.maxTokensField = "max_tokens"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
				w.supportsStrictMode = false
			}),
		},
		{
			name:  "ant-ling",
			model: llm.Model{Provider: "ant-ling", ID: "ling-v1"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "ant-ling"
				w.maxTokensField = "max_tokens"
				w.supportsStore = false
				w.supportsDeveloperRole = false
				w.supportsReasoningEffort = false
			}),
		},
		{
			name:  "openrouter generic model has no developer role and routes to openrouter thinking",
			model: llm.Model{Provider: "openrouter", ID: "google/gemini-pro"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "openrouter"
				w.supportsDeveloperRole = false
			}),
		},
		{
			name:  "openrouter anthropic-prefixed model regains developer role",
			model: llm.Model{Provider: "openrouter", ID: "anthropic/claude-3.5"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "openrouter"
			}),
		},
		{
			name:  "openrouter openai-prefixed model regains developer role",
			model: llm.Model{Provider: "openrouter", ID: "openai/gpt-4"},
			want: override(defaults, func(w *want) {
				w.thinkingFormat = "openrouter"
			}),
		},
		{
			name:  "cerebras",
			model: llm.Model{Provider: "cerebras", ID: "llama-cerebras"},
			want: override(defaults, func(w *want) {
				w.supportsStore = false
				w.supportsDeveloperRole = false
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectCompat(test.model)
			if got.thinkingFormat != test.want.thinkingFormat {
				t.Errorf("thinkingFormat = %q, want %q", got.thinkingFormat, test.want.thinkingFormat)
			}
			if got.maxTokensField != test.want.maxTokensField {
				t.Errorf("maxTokensField = %q, want %q", got.maxTokensField, test.want.maxTokensField)
			}
			if got.supportsReasoningEffort != test.want.supportsReasoningEffort {
				t.Errorf("supportsReasoningEffort = %v, want %v", got.supportsReasoningEffort, test.want.supportsReasoningEffort)
			}
			if got.supportsStore != test.want.supportsStore {
				t.Errorf("supportsStore = %v, want %v", got.supportsStore, test.want.supportsStore)
			}
			if got.supportsDeveloperRole != test.want.supportsDeveloperRole {
				t.Errorf("supportsDeveloperRole = %v, want %v", got.supportsDeveloperRole, test.want.supportsDeveloperRole)
			}
			if got.supportsStrictMode != test.want.supportsStrictMode {
				t.Errorf("supportsStrictMode = %v, want %v", got.supportsStrictMode, test.want.supportsStrictMode)
			}
			if got.requiresReasoningContentOnAssistantMessages != test.want.requiresReasoningOnAsst {
				t.Errorf("requiresReasoningContentOnAssistantMessages = %v, want %v",
					got.requiresReasoningContentOnAssistantMessages, test.want.requiresReasoningOnAsst)
			}
		})
	}
}

func TestDetectCompatMatchesByBaseURL(t *testing.T) {
	// A baseURL substring must trigger the same provider detection as the
	// provider id, so a generic model id pointed at a known endpoint inherits
	// that provider's quirks.
	tests := []struct {
		name       string
		baseURL    string
		want       string // thinkingFormat
		maxTokens  string // maxTokensField
		strictMode bool
	}{
		{name: "z.ai", baseURL: "https://api.z.ai/v1", want: "zai", maxTokens: "max_completion_tokens", strictMode: true},
		{name: "bigmodel", baseURL: "https://open.bigmodel.cn/api", want: "zai", maxTokens: "max_completion_tokens", strictMode: true},
		{name: "together.ai", baseURL: "https://api.together.ai/v1", want: "together", maxTokens: "max_tokens", strictMode: false},
		{name: "together.xyz", baseURL: "https://api.together.xyz/v1", want: "together", maxTokens: "max_tokens", strictMode: false},
		{name: "moonshot global", baseURL: "https://api.moonshot.ai/v1", want: "openai", maxTokens: "max_tokens", strictMode: false},
		{name: "moonshot cn", baseURL: "https://api.moonshot.cn/v1", want: "openai", maxTokens: "max_tokens", strictMode: false},
		{name: "openrouter", baseURL: "https://openrouter.ai/api/v1", want: "openrouter", maxTokens: "max_completion_tokens", strictMode: true},
		{name: "deepseek", baseURL: "https://api.deepseek.com/v1", want: "deepseek", maxTokens: "max_completion_tokens", strictMode: true},
		{name: "grok via x.ai", baseURL: "https://api.x.ai/v1", want: "openai", maxTokens: "max_completion_tokens", strictMode: true},
		{name: "chutes", baseURL: "https://chutes.ai/v1", want: "openai", maxTokens: "max_tokens", strictMode: true},
		{name: "cloudflare ai gateway", baseURL: "https://gateway.ai.cloudflare.com/v1", want: "openai", maxTokens: "max_tokens", strictMode: false},
		{name: "cloudflare workers ai", baseURL: "https://api.cloudflare.com/v1", want: "openai", maxTokens: "max_completion_tokens", strictMode: true},
		{name: "ant-ling", baseURL: "https://api.ant-ling.com/v1", want: "ant-ling", maxTokens: "max_tokens", strictMode: true},
		{name: "nvidia", baseURL: "https://integrate.api.nvidia.com/v1", want: "openai", maxTokens: "max_tokens", strictMode: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectCompat(llm.Model{Provider: "unknown", ID: "model-x", BaseURL: test.baseURL})
			if got.thinkingFormat != test.want {
				t.Errorf("thinkingFormat = %q, want %q", got.thinkingFormat, test.want)
			}
			if got.maxTokensField != test.maxTokens {
				t.Errorf("maxTokensField = %q, want %q", got.maxTokensField, test.maxTokens)
			}
			if got.supportsStrictMode != test.strictMode {
				t.Errorf("supportsStrictMode = %v, want %v", got.supportsStrictMode, test.strictMode)
			}
		})
	}
}

func TestResolveCompatWithoutOverride(t *testing.T) {
	// No compatibility set: result equals detection.
	model := llm.Model{Provider: "openai", ID: "gpt-x"}
	got := resolveCompat(model)
	want := detectCompat(model)
	if got != want {
		t.Fatalf("resolveCompat = %+v, want %+v", got, want)
	}
}

func TestResolveCompatNilCompatibility(t *testing.T) {
	// A nil *OpenAICompletionsCompatibility carried in the interface must not
	// crash and must leave detection untouched.
	model := llm.Model{
		Provider:      "openai",
		ID:            "gpt-x",
		Compatibility: (*llm.OpenAICompletionsCompatibility)(nil),
	}
	got := resolveCompat(model)
	want := detectCompat(model)
	if got != want {
		t.Fatalf("nil compat resolve = %+v, want %+v", got, want)
	}
}

func TestResolveCompatAppliesAllOverrides(t *testing.T) {
	// Every override field flips the detection default so the test fails if any
	// one of them stops being applied.
	model := llm.Model{
		Provider: "openai",
		ID:       "gpt-x",
		Compatibility: &llm.OpenAICompletionsCompatibility{
			SupportsStore:                               boolPtr(false),
			SupportsDeveloperRole:                       boolPtr(false),
			SupportsReasoningEffort:                     boolPtr(false),
			MaxTokensField:                              "max_tokens",
			SupportsStrictMode:                          boolPtr(false),
			RequiresReasoningContentOnAssistantMessages: boolPtr(true),
			RequiresThinkingAsText:                      boolPtr(true),
			ThinkingFormat:                              "qwen",
			ZAIToolStream:                               boolPtr(true),
		},
	}
	got := resolveCompat(model)

	if got.supportsStore {
		t.Errorf("SupportsStore override not applied")
	}
	if got.supportsDeveloperRole {
		t.Errorf("SupportsDeveloperRole override not applied")
	}
	if got.supportsReasoningEffort {
		t.Errorf("SupportsReasoningEffort override not applied")
	}
	if got.maxTokensField != "max_tokens" {
		t.Errorf("MaxTokensField = %q, want max_tokens", got.maxTokensField)
	}
	if got.supportsStrictMode {
		t.Errorf("SupportsStrictMode override not applied")
	}
	if !got.requiresReasoningContentOnAssistantMessages {
		t.Errorf("RequiresReasoningContentOnAssistantMessages override not applied")
	}
	if !got.requiresThinkingAsText {
		t.Errorf("RequiresThinkingAsText override not applied")
	}
	if got.thinkingFormat != "qwen" {
		t.Errorf("ThinkingFormat = %q, want qwen", got.thinkingFormat)
	}
	if !got.zaiToolStream {
		t.Errorf("ZAIToolStream override not applied")
	}
}

func TestResolveCompatEmptyFieldsKeepDetected(t *testing.T) {
	// An override with nil pointers and empty strings must not overwrite the
	// detected values: pointer booleans encode "unspecified" as nil.
	model := llm.Model{
		Provider:      "deepseek",
		ID:            "deepseek-r1",
		Compatibility: &llm.OpenAICompletionsCompatibility{},
	}
	got := resolveCompat(model)
	want := detectCompat(model)
	if got != want {
		t.Fatalf("empty override changed compat: got %+v want %+v", got, want)
	}
}

func TestResolveCompatRejectsWrongProtocolCompat(t *testing.T) {
	// An Anthropic compatibility struct attached to an OpenAI model must be
	// ignored rather than panic, so detection wins.
	model := llm.Model{
		Provider:      "openai",
		ID:            "gpt-x",
		Compatibility: &llm.AnthropicMessagesCompatibility{},
	}
	got := resolveCompat(model)
	want := detectCompat(model)
	if got != want {
		t.Fatalf("wrong-protocol override = %+v, want detection %+v", got, want)
	}
}
