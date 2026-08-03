package openai

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
)

func reasoningModel(levels map[llm.ModelThinkingLevel]*string) llm.Model {
	return llm.Model{
		ID:               "test-model",
		Protocol:         llm.ProtocolOpenAICompletions,
		Provider:         "test",
		Reasoning:        true,
		ThinkingLevelMap: levels,
	}
}

func strPtr(s string) *string { return &s }

func explicitThinking(level llm.ModelThinkingLevel) resolvedThinking {
	return resolvedThinking{specified: true, level: level}
}

// extraFields encodes params and returns the decoded ExtraFields map so tests can
// assert on non-standard reasoning fields written through SetExtraFields.
func extraFields(t *testing.T, params oai.ChatCompletionNewParams) map[string]any {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	return decoded
}

var thinkingControlFields = []string{
	"reasoning_effort",
	"thinking",
	"reasoning",
	"enable_thinking",
	"chat_template_kwargs",
	"output_config",
}

func thinkingControls(t *testing.T, params oai.ChatCompletionNewParams) map[string]any {
	t.Helper()
	body := extraFields(t, params)
	controls := make(map[string]any)
	for _, field := range thinkingControlFields {
		if value, ok := body[field]; ok {
			controls[field] = value
		}
	}
	return controls
}

type thinkingWireCase struct {
	name      string
	reasoning llm.ModelThinkingLevel
	want      map[string]any
}

func runThinkingWireCases(
	t *testing.T,
	model llm.Model,
	compat resolvedCompat,
	cases []thinkingWireCase,
) {
	t.Helper()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			params := buildParams(
				model,
				nil,
				nil,
				llm.StreamOptions{Reasoning: test.reasoning},
				compat,
			)
			if got := thinkingControls(t, params); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("thinking controls = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestResolveThinking(t *testing.T) {
	mediumValue := "medium"
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingMedium: &mediumValue,
	})

	if got := resolveThinking(model, ""); got.specified {
		t.Fatalf("empty request = %#v, want unspecified", got)
	}

	if got := resolveThinking(model, llm.ModelThinkingOff); !got.specified || got.level != llm.ModelThinkingOff {
		t.Fatalf("off request = %#v, want explicit off", got)
	}

	if got := resolveThinking(model, llm.ModelThinkingMedium); !got.specified || got.level != llm.ModelThinkingMedium {
		t.Fatalf("medium request = %#v, want explicit medium", got)
	}

	// A non-reasoning model clamps any explicit request down to off without
	// turning it into an unspecified request.
	plain := llm.Model{Reasoning: false}
	if got := resolveThinking(plain, llm.ModelThinkingHigh); !got.specified || got.level != llm.ModelThinkingOff {
		t.Fatalf("non-reasoning model returns %#v, want explicit off", got)
	}
}

func TestBuildParamsStandardOpenAIThinkingWireMatrix(t *testing.T) {
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff:  strPtr("none"),
		llm.ModelThinkingHigh: strPtr("high"),
		llm.ModelThinkingMax:  strPtr("max"),
	})
	runThinkingWireCases(t, model, resolvedCompat{
		thinkingFormat:          "openai",
		supportsReasoningEffort: true,
	}, []thinkingWireCase{
		{name: "unset", want: map[string]any{}},
		{
			name:      "off",
			reasoning: llm.ModelThinkingOff,
			want:      map[string]any{"reasoning_effort": "none"},
		},
		{
			name:      "high",
			reasoning: llm.ModelThinkingHigh,
			want:      map[string]any{"reasoning_effort": "high"},
		},
		{
			name:      "max",
			reasoning: llm.ModelThinkingMax,
			want:      map[string]any{"reasoning_effort": "max"},
		},
	})
}

func TestBuildParamsUnsetOmitsThinkingControlsForEveryFormat(t *testing.T) {
	model := reasoningModel(nil)
	for _, format := range []string{
		"openai",
		"zai",
		"qwen",
		"qwen-chat-template",
		"xiaomi",
		"deepseek",
		"ant-ling",
		"together",
		"string-thinking",
	} {
		t.Run(format, func(t *testing.T) {
			params := buildParams(model, nil, nil, llm.StreamOptions{}, resolvedCompat{
				thinkingFormat:          format,
				supportsReasoningEffort: true,
			})
			if got := thinkingControls(t, params); len(got) != 0 {
				t.Fatalf("unset thinking controls = %#v, want none", got)
			}
		})
	}
}

func TestBuildParamsProviderThinkingWireMatrix(t *testing.T) {
	disabled := map[string]any{"type": "disabled"}
	enabled := map[string]any{"type": "enabled"}
	zaiEnabled := map[string]any{"type": "enabled", "clear_thinking": false}

	tests := []struct {
		name     string
		provider string
		modelID  string
		cases    []thinkingWireCase
	}{
		{
			name:     "deepseek direct",
			provider: "deepseek",
			modelID:  "deepseek-v4-flash",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": enabled, "reasoning_effort": "high"}},
				{name: "max", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": enabled, "reasoning_effort": "max"}},
			},
		},
		{
			name:     "moonshot kimi k3 effort",
			provider: "moonshotai",
			modelID:  "kimi-k3",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps low", reasoning: llm.ModelThinkingOff, want: map[string]any{"reasoning_effort": "low"}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"reasoning_effort": "high"}},
				{name: "max", reasoning: llm.ModelThinkingMax, want: map[string]any{"reasoning_effort": "max"}},
			},
		},
		{
			name:     "moonshot kimi k2.6 toggle",
			provider: "moonshotai",
			modelID:  "kimi-k2.6",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": enabled}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": enabled}},
			},
		},
		{
			name:     "moonshot kimi k2.7 always thinking",
			provider: "moonshotai",
			modelID:  "kimi-k2.7-code",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps enabled", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": enabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": enabled}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": enabled}},
			},
		},
		{
			name:     "opencode zen deepseek",
			provider: "opencode",
			modelID:  "deepseek-v4-flash",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps high", reasoning: llm.ModelThinkingOff, want: map[string]any{"reasoning_effort": "high"}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"reasoning_effort": "high"}},
				{name: "max", reasoning: llm.ModelThinkingMax, want: map[string]any{"reasoning_effort": "max"}},
			},
		},
		{
			name:     "opencode go deepseek",
			provider: "opencode-go",
			modelID:  "deepseek-v4-flash",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": enabled, "reasoning_effort": "high"}},
				{name: "max", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": enabled, "reasoning_effort": "max"}},
			},
		},
		{
			name:     "opencode kimi toggle",
			provider: "opencode",
			modelID:  "kimi-k2.6",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": enabled}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": enabled}},
			},
		},
		{
			name:     "opencode go kimi toggle",
			provider: "opencode-go",
			modelID:  "kimi-k2.6",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": enabled}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": enabled}},
			},
		},
		{
			name:     "opencode go minimax m2.7 fixed hidden reasoning",
			provider: "opencode-go",
			modelID:  "minimax-m2.7",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps high", reasoning: llm.ModelThinkingOff, want: map[string]any{}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{}},
			},
		},
		{
			name:     "opencode go glm always thinking",
			provider: "opencode-go",
			modelID:  "glm-5.2",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps high", reasoning: llm.ModelThinkingOff, want: map[string]any{"reasoning_effort": "high"}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"reasoning_effort": "high"}},
				{name: "max", reasoning: llm.ModelThinkingMax, want: map[string]any{"reasoning_effort": "max"}},
			},
		},
		{
			name:     "opencode grok server default",
			provider: "opencode",
			modelID:  "grok-build-0.1",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps high", reasoning: llm.ModelThinkingOff, want: map[string]any{}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{}},
			},
		},
		{
			name:     "together fixed reasoning",
			provider: "together",
			modelID:  "MiniMaxAI/MiniMax-M2.7",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps high", reasoning: llm.ModelThinkingOff, want: map[string]any{}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{}},
			},
		},
		{
			name:     "together reasoning effort",
			provider: "together",
			modelID:  "openai/gpt-oss-120b",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off clamps low", reasoning: llm.ModelThinkingOff, want: map[string]any{"reasoning_effort": "low"}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"reasoning_effort": "high"}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"reasoning_effort": "high"}},
			},
		},
		{
			name:     "together toggle and effort",
			provider: "together",
			modelID:  "deepseek-ai/DeepSeek-V4-Pro",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"reasoning": map[string]any{"enabled": false}}},
				{
					name: "high", reasoning: llm.ModelThinkingHigh,
					want: map[string]any{"reasoning": map[string]any{"enabled": true}, "reasoning_effort": "high"},
				},
				{
					name: "max clamps high", reasoning: llm.ModelThinkingMax,
					want: map[string]any{"reasoning": map[string]any{"enabled": true}, "reasoning_effort": "high"},
				},
			},
		},
		{
			name:     "together toggle only",
			provider: "together",
			modelID:  "moonshotai/Kimi-K2.6",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"reasoning": map[string]any{"enabled": false}}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"reasoning": map[string]any{"enabled": true}}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"reasoning": map[string]any{"enabled": true}}},
			},
		},
		{
			name:     "zai glm toggle",
			provider: "zai",
			modelID:  "glm-4.7",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": zaiEnabled}},
				{name: "max clamps high", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": zaiEnabled}},
			},
		},
		{
			name:     "zai glm",
			provider: "zai",
			modelID:  "glm-5.2",
			cases: []thinkingWireCase{
				{name: "unset", want: map[string]any{}},
				{name: "off", reasoning: llm.ModelThinkingOff, want: map[string]any{"thinking": disabled}},
				{name: "high", reasoning: llm.ModelThinkingHigh, want: map[string]any{"thinking": zaiEnabled, "reasoning_effort": "high"}},
				{name: "max", reasoning: llm.ModelThinkingMax, want: map[string]any{"thinking": zaiEnabled, "reasoning_effort": "max"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, ok := llm.LookupModel(test.provider, test.modelID)
			if !ok {
				t.Fatalf("model %s/%s is missing from the catalog", test.provider, test.modelID)
			}
			runThinkingWireCases(t, model, resolveCompat(model), test.cases)
		})
	}
}

func TestApplyThinkingNonReasoningModelIsNoop(t *testing.T) {
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, llm.Model{Reasoning: false}, resolvedCompat{thinkingFormat: "openai"}, explicitThinking(llm.ModelThinkingHigh))
	if len(params.ExtraFields()) != 0 {
		t.Fatalf("non-reasoning model wrote extras: %#v", params.ExtraFields())
	}
}

func TestApplyThinkingOpenAIDefault(t *testing.T) {
	// Default OpenAI format with an effort writes reasoning_effort.
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: true}, explicitThinking(llm.ModelThinkingHigh))

	if got := extraFields(t, params)["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
}

func TestApplyThinkingOpenAIWithoutEffortUsesOffString(t *testing.T) {
	// When the model maps off to a concrete string and no effort is requested,
	// the off mapping is sent so the provider sees thinking disabled.
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: strPtr("none"),
	})
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: true}, explicitThinking(llm.ModelThinkingOff))

	if got := extraFields(t, params)["reasoning_effort"]; got != "none" {
		t.Fatalf("reasoning_effort = %#v, want none", got)
	}
}

func TestApplyThinkingOpenAIWithoutEffortNoOffString(t *testing.T) {
	// Without a concrete off mapping, the default OpenAI branch writes nothing
	// rather than guess.
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: true}, explicitThinking(llm.ModelThinkingOff))

	if got := extraFields(t, params)["reasoning_effort"]; got != nil {
		t.Fatalf("reasoning_effort = %#v, want absent", got)
	}
}

func TestApplyThinkingOpenAIIgnoresEffortWhenUnsupported(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: false}, explicitThinking(llm.ModelThinkingHigh))

	if got := extraFields(t, params)["reasoning_effort"]; got != nil {
		t.Fatalf("reasoning_effort = %#v, want absent when unsupported", got)
	}
}

func TestApplyThinkingZAIEnabled(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "zai", supportsReasoningEffort: true}, explicitThinking(llm.ModelThinkingHigh))

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["thinking"], map[string]any{"type": "enabled", "clear_thinking": false}) {
		t.Fatalf("thinking = %#v, want enabled with preservation", extras["thinking"])
	}
	if got := extras["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
}

func TestApplyThinkingZAIDisabled(t *testing.T) {
	// ZAI without an effort writes the disabled thinking type and never sets
	// reasoning_effort.
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "zai"}, explicitThinking(llm.ModelThinkingOff))

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["thinking"], map[string]any{"type": "disabled"}) {
		t.Fatalf("thinking = %#v, want disabled", extras["thinking"])
	}
	if _, present := extras["reasoning_effort"]; present {
		t.Fatalf("reasoning_effort must be absent: %#v", extras)
	}
}

func TestApplyThinkingQwen(t *testing.T) {
	model := reasoningModel(nil)
	for _, enabled := range []bool{true, false} {
		thinking := explicitThinking(llm.ModelThinkingHigh)
		if !enabled {
			thinking = explicitThinking(llm.ModelThinkingOff)
		}
		params := oai.ChatCompletionNewParams{}
		applyThinking(&params, model, resolvedCompat{thinkingFormat: "qwen"}, thinking)
		if got := extraFields(t, params)["enable_thinking"]; got != enabled {
			t.Fatalf("enable_thinking(%v) = %#v", enabled, got)
		}
	}
}

func TestApplyThinkingQwenChatTemplate(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "qwen-chat-template"}, explicitThinking(llm.ModelThinkingHigh))

	got := extraFields(t, params)["chat_template_kwargs"]
	want := map[string]any{"enable_thinking": true, "preserve_thinking": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chat_template_kwargs = %#v, want %#v", got, want)
	}
}

func TestBuildParamsClampsUnsupportedDeepSeekOff(t *testing.T) {
	// An explicit off request is clamped to the nearest supported level before
	// request serialization. It must not emit a disable parameter the model
	// rejects.
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	})
	params := buildParams(
		model,
		nil,
		nil,
		llm.StreamOptions{Reasoning: llm.ModelThinkingOff},
		resolvedCompat{thinkingFormat: "deepseek"},
	)

	if got := extraFields(t, params)["thinking"]; !reflect.DeepEqual(got, map[string]any{"type": "enabled"}) {
		t.Fatalf("thinking = %#v, want enabled after clamp", got)
	}
}

func TestOpenCodeThinkingOffPayloads(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		modelID  string
		field    string
		want     any
	}{
		{
			name:     "opencode go deepseek",
			provider: "opencode-go", modelID: "deepseek-v4-flash",
			field: "thinking", want: map[string]any{"type": "disabled"},
		},
		{
			name:     "opencode kimi",
			provider: "opencode", modelID: "kimi-k2.6",
			field: "thinking", want: map[string]any{"type": "disabled"},
		},
		{
			name:     "opencode go kimi",
			provider: "opencode-go", modelID: "kimi-k2.6",
			field: "thinking", want: map[string]any{"type": "disabled"},
		},
		{
			name:     "opencode go mimo",
			provider: "opencode-go", modelID: "mimo-v2.5",
			field: "thinking", want: map[string]any{"type": "disabled"},
		},
		{
			name:     "opencode go qwen",
			provider: "opencode-go", modelID: "qwen3.6-plus",
			field: "enable_thinking", want: false,
		},
		{
			name:     "opencode go hy3",
			provider: "opencode-go", modelID: "hy3",
			field: "reasoning_effort", want: "none",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, ok := llm.LookupModel(test.provider, test.modelID)
			if !ok {
				t.Fatalf("model %s/%s is missing from the catalog", test.provider, test.modelID)
			}
			params := oai.ChatCompletionNewParams{}
			applyThinking(&params, model, resolveCompat(model), resolveThinking(model, llm.ModelThinkingOff))

			extras := extraFields(t, params)
			if got := extras[test.field]; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("%s = %#v, want %#v", test.field, got, test.want)
			}
			if test.field != "reasoning_effort" {
				if _, present := extras["reasoning_effort"]; present {
					t.Fatalf("reasoning_effort must be absent when thinking is off: %#v", extras)
				}
			}
		})
	}
}

func TestOpenCodeModelsAdvertisingOffAlwaysSendAControl(t *testing.T) {
	for _, provider := range []string{"opencode", "opencode-go"} {
		for _, model := range llm.GetModels(provider) {
			if model.Protocol != llm.ProtocolOpenAICompletions ||
				!model.Reasoning ||
				!slices.Contains(llm.SupportedThinkingLevels(model), llm.ModelThinkingOff) {
				continue
			}

			params := buildParams(
				model,
				nil,
				nil,
				llm.StreamOptions{Reasoning: llm.ModelThinkingOff},
				resolveCompat(model),
			)
			if controls := thinkingControls(t, params); len(controls) == 0 {
				t.Errorf("%s/%s advertises thinking Off but sends no control", provider, model.ID)
			}
		}
	}
}

func TestOpenCodeHy3ThinkingLevels(t *testing.T) {
	model, ok := llm.LookupModel("opencode-go", "hy3")
	if !ok {
		t.Fatal("model opencode-go/hy3 is missing from the catalog")
	}
	want := []llm.ModelThinkingLevel{
		llm.ModelThinkingOff,
		llm.ModelThinkingLow,
		llm.ModelThinkingHigh,
	}
	if got := llm.SupportedThinkingLevels(model); !reflect.DeepEqual(got, want) {
		t.Fatalf("thinking levels = %v, want %v", got, want)
	}
}

func TestOpenCodeKimiThinkingLevels(t *testing.T) {
	want := []llm.ModelThinkingLevel{
		llm.ModelThinkingOff,
		llm.ModelThinkingHigh,
	}
	for _, provider := range []string{"opencode", "opencode-go"} {
		model, ok := llm.LookupModel(provider, "kimi-k2.6")
		if !ok {
			t.Fatalf("model %s/kimi-k2.6 is missing from the catalog", provider)
		}
		if got := llm.SupportedThinkingLevels(model); !reflect.DeepEqual(got, want) {
			t.Errorf("%s/kimi-k2.6 thinking levels = %v, want %v", provider, got, want)
		}
	}
}

func TestOpenCodeQwenThinkingLevels(t *testing.T) {
	model, ok := llm.LookupModel("opencode-go", "qwen3.6-plus")
	if !ok {
		t.Fatal("model opencode-go/qwen3.6-plus is missing from the catalog")
	}
	want := []llm.ModelThinkingLevel{
		llm.ModelThinkingOff,
		llm.ModelThinkingHigh,
	}
	if got := llm.SupportedThinkingLevels(model); !reflect.DeepEqual(got, want) {
		t.Fatalf("thinking levels = %v, want %v", got, want)
	}
}

func TestOpenCodeMiMoThinkingLevels(t *testing.T) {
	tests := []struct {
		modelID string
		want    []llm.ModelThinkingLevel
	}{
		{modelID: "mimo-v2.5", want: []llm.ModelThinkingLevel{llm.ModelThinkingOff, llm.ModelThinkingHigh}},
		{modelID: "mimo-v2.5-pro", want: []llm.ModelThinkingLevel{llm.ModelThinkingHigh}},
	}

	for _, test := range tests {
		model, ok := llm.LookupModel("opencode-go", test.modelID)
		if !ok {
			t.Fatalf("model opencode-go/%s is missing from the catalog", test.modelID)
		}
		if got := llm.SupportedThinkingLevels(model); !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s thinking levels = %v, want %v", test.modelID, got, test.want)
		}
	}
}

func TestGeneratedDirectEffortThinkingLevels(t *testing.T) {
	want := []llm.ModelThinkingLevel{
		llm.ModelThinkingLow,
		llm.ModelThinkingMedium,
		llm.ModelThinkingHigh,
	}
	for _, ref := range []struct {
		provider string
		modelID  string
	}{
		{provider: "cerebras", modelID: "gpt-oss-120b"},
		{provider: "groq", modelID: "openai/gpt-oss-120b"},
	} {
		model, ok := llm.LookupModel(ref.provider, ref.modelID)
		if !ok {
			t.Fatalf("model %s/%s is missing from the catalog", ref.provider, ref.modelID)
		}
		if got := llm.SupportedThinkingLevels(model); !reflect.DeepEqual(got, want) {
			t.Errorf("%s/%s thinking levels = %v, want %v", ref.provider, ref.modelID, got, want)
		}
	}
}

func TestGeneratedXiaomiToggleThinking(t *testing.T) {
	wantLevels := []llm.ModelThinkingLevel{
		llm.ModelThinkingOff,
		llm.ModelThinkingHigh,
	}
	for _, ref := range []struct {
		provider string
		modelID  string
	}{
		{provider: "xiaomi", modelID: "mimo-v2.5"},
		{provider: "xiaomi", modelID: "mimo-v2.5-pro"},
		{provider: "xiaomi", modelID: "mimo-v2.5-pro-ultraspeed"},
		{provider: "xiaomi-token-plan-cn", modelID: "mimo-v2.5"},
		{provider: "xiaomi-token-plan-cn", modelID: "mimo-v2.5-pro"},
		{provider: "xiaomi-token-plan-ams", modelID: "mimo-v2.5"},
		{provider: "xiaomi-token-plan-ams", modelID: "mimo-v2.5-pro"},
		{provider: "xiaomi-token-plan-sgp", modelID: "mimo-v2.5"},
		{provider: "xiaomi-token-plan-sgp", modelID: "mimo-v2.5-pro"},
	} {
		t.Run(ref.provider+"/"+ref.modelID, func(t *testing.T) {
			model, ok := llm.LookupModel(ref.provider, ref.modelID)
			if !ok {
				t.Fatal("verified Xiaomi toggle route is missing from the catalog")
			}
			if got := llm.SupportedThinkingLevels(model); !reflect.DeepEqual(got, wantLevels) {
				t.Errorf("thinking levels = %v, want %v", got, wantLevels)
			}

			compat := resolveCompat(model)
			if compat.thinkingFormat != "xiaomi" || compat.supportsReasoningEffort {
				t.Errorf(
					"compatibility = {format:%q effort:%v}, want {xiaomi false}",
					compat.thinkingFormat,
					compat.supportsReasoningEffort,
				)
			}
			for _, test := range []struct {
				level llm.ModelThinkingLevel
				want  string
			}{
				{level: llm.ModelThinkingOff, want: "disabled"},
				{level: llm.ModelThinkingHigh, want: "enabled"},
			} {
				params := oai.ChatCompletionNewParams{}
				applyThinking(&params, model, compat, explicitThinking(test.level))
				extras := extraFields(t, params)
				if !reflect.DeepEqual(extras["thinking"], map[string]any{"type": test.want}) {
					t.Errorf("%s thinking = %#v, want %s", test.level, extras["thinking"], test.want)
				}
				if _, present := extras["reasoning_effort"]; present {
					t.Errorf("%s unexpectedly sends reasoning_effort: %#v", test.level, extras)
				}
			}
		})
	}
}

func TestGeneratedThinkingDialectIsolation(t *testing.T) {
	tests := []struct {
		provider string
		modelID  string
		want     string
	}{
		{provider: "xiaomi", modelID: "mimo-v2.5", want: "xiaomi"},
		{provider: "opencode-go", modelID: "mimo-v2.5", want: "deepseek"},
		{provider: "opencode-go", modelID: "mimo-v2.5-pro", want: "openai"},
		{provider: "deepseek", modelID: "deepseek-v4-flash", want: "deepseek"},
		{provider: "opencode", modelID: "deepseek-v4-flash", want: "openai"},
		{provider: "opencode-go", modelID: "deepseek-v4-flash", want: "deepseek"},
		{provider: "together", modelID: "deepseek-ai/DeepSeek-V4-Pro", want: "together"},
	}

	for _, test := range tests {
		t.Run(test.provider+"/"+test.modelID, func(t *testing.T) {
			model, ok := llm.LookupModel(test.provider, test.modelID)
			if !ok {
				t.Fatalf("model is missing from the catalog")
			}
			if got := resolveCompat(model).thinkingFormat; got != test.want {
				t.Fatalf("thinking format = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOpenCodeAlwaysThinkingModelsExcludeOff(t *testing.T) {
	tests := []struct {
		provider string
		modelID  string
	}{
		{provider: "opencode-go", modelID: "glm-5.2"},
		{provider: "opencode-go", modelID: "mimo-v2.5-pro"},
		{provider: "opencode", modelID: "grok-build-0.1"},
		{provider: "opencode", modelID: "glm-5"},
		{provider: "opencode", modelID: "deepseek-v4-flash"},
	}

	for _, test := range tests {
		model, ok := llm.LookupModel(test.provider, test.modelID)
		if !ok {
			t.Fatalf("model %s/%s is missing from the catalog", test.provider, test.modelID)
		}
		for _, level := range llm.SupportedThinkingLevels(model) {
			if level == llm.ModelThinkingOff {
				t.Errorf("%s/%s unexpectedly supports thinking off", test.provider, test.modelID)
			}
		}
	}
}

func TestApplyThinkingAntLingOnlyWhenLevelMapped(t *testing.T) {
	// ant-ling sends reasoning only when the effort level is explicitly mapped.
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingHigh: strPtr("hard"),
	})
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "ant-ling"}, explicitThinking(llm.ModelThinkingHigh))

	got := extraFields(t, params)["reasoning"]
	want := map[string]any{"effort": "hard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reasoning = %#v, want %#v", got, want)
	}
}

func TestApplyThinkingAntLingSkipsUnmappedLevel(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "ant-ling"}, explicitThinking(llm.ModelThinkingHigh))

	if _, present := extraFields(t, params)["reasoning"]; present {
		t.Fatalf("ant-ling must skip unmapped levels")
	}
}

func TestApplyThinkingTogether(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "together", supportsReasoningEffort: true}, explicitThinking(llm.ModelThinkingHigh))

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["reasoning"], map[string]any{"enabled": true}) {
		t.Fatalf("reasoning = %#v, want enabled=true", extras["reasoning"])
	}
	if got := extras["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
}

func TestApplyThinkingTogetherDisabledOmitsReasoningEffort(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "together"}, explicitThinking(llm.ModelThinkingOff))

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["reasoning"], map[string]any{"enabled": false}) {
		t.Fatalf("reasoning = %#v, want enabled=false", extras["reasoning"])
	}
	if _, present := extras["reasoning_effort"]; present {
		t.Fatalf("reasoning_effort must be absent when disabled: %#v", extras)
	}
}

func TestApplyThinkingStringFormat(t *testing.T) {
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingHigh: strPtr("deep"),
		llm.ModelThinkingOff:  strPtr("off"),
	})

	on := oai.ChatCompletionNewParams{}
	applyThinking(&on, model, resolvedCompat{thinkingFormat: "string-thinking"}, explicitThinking(llm.ModelThinkingHigh))
	if got := extraFields(t, on)["thinking"]; got != "deep" {
		t.Fatalf("thinking on = %#v, want deep", got)
	}

	off := oai.ChatCompletionNewParams{}
	applyThinking(&off, model, resolvedCompat{thinkingFormat: "string-thinking"}, explicitThinking(llm.ModelThinkingOff))
	if got := extraFields(t, off)["thinking"]; got != "off" {
		t.Fatalf("thinking off = %#v, want off", got)
	}
}

func TestBuildParamsClampsUnsupportedStringThinkingOff(t *testing.T) {
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	})
	params := buildParams(
		model,
		nil,
		nil,
		llm.StreamOptions{Reasoning: llm.ModelThinkingOff},
		resolvedCompat{thinkingFormat: "string-thinking"},
	)

	if got := extraFields(t, params)["thinking"]; got != "minimal" {
		t.Fatalf("thinking = %#v, want minimal after clamp", got)
	}
}

func TestThinkingTypeHelper(t *testing.T) {
	if got := thinkingType(true); !reflect.DeepEqual(got, map[string]any{"type": "enabled"}) {
		t.Fatalf("thinkingType(true) = %#v", got)
	}
	if got := thinkingType(false); !reflect.DeepEqual(got, map[string]any{"type": "disabled"}) {
		t.Fatalf("thinkingType(false) = %#v", got)
	}
	if got := zaiThinkingType(true); !reflect.DeepEqual(got, map[string]any{"type": "enabled", "clear_thinking": false}) {
		t.Fatalf("zaiThinkingType(true) = %#v", got)
	}
}

func TestMappedEffortFallsBackToLevelName(t *testing.T) {
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingHigh:   strPtr("hard"),
		llm.ModelThinkingMedium: nil,
	})

	if got := mappedEffort(model, llm.ModelThinkingHigh); got != "hard" {
		t.Fatalf("mapped high = %q, want hard", got)
	}
	// Missing entry falls back to the level string.
	if got := mappedEffort(model, llm.ModelThinkingLow); got != "low" {
		t.Fatalf("missing low = %q, want low", got)
	}
	// Explicit nil mapping also falls back: the helper has no way to express
	// "unmapped", and callers gate on the nil case before calling.
	if got := mappedEffort(model, llm.ModelThinkingMedium); got != "medium" {
		t.Fatalf("nil-mapped medium = %q, want medium", got)
	}
}

func TestOffEffortHelpers(t *testing.T) {
	withOff := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: strPtr("disabled"),
	})
	withoutOff := reasoningModel(nil)

	if got := offEffort(withOff); got != "disabled" {
		t.Fatalf("offEffort mapped = %q", got)
	}
	if got := offEffort(withoutOff); got != "none" {
		t.Fatalf("offEffort default = %q, want none", got)
	}

	if value, ok := offString(withOff); !ok || value != "disabled" {
		t.Fatalf("offString mapped = %q %v", value, ok)
	}
	if _, ok := offString(withoutOff); ok {
		t.Fatalf("offString without mapping must report false")
	}
}
