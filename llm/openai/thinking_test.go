package openai

import (
	"encoding/json"
	"reflect"
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

func TestResolveEffort(t *testing.T) {
	mediumValue := "medium"
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingMedium: &mediumValue,
	})

	if got := resolveEffort(model, ""); got != "" {
		t.Fatalf("empty request = %q, want \"\"", got)
	}

	if got := resolveEffort(model, llm.ModelThinkingOff); got != "" {
		t.Fatalf("off request = %q, want \"\"", got)
	}

	if got := resolveEffort(model, llm.ModelThinkingMedium); got != llm.ModelThinkingMedium {
		t.Fatalf("medium request = %q, want medium", got)
	}

	// A non-reasoning model clamps any request down to off, which resolveEffort
	// returns as "".
	plain := llm.Model{Reasoning: false}
	if got := resolveEffort(plain, llm.ModelThinkingHigh); got != "" {
		t.Fatalf("non-reasoning model returns %q, want \"\"", got)
	}
}

func TestApplyThinkingNonReasoningModelIsNoop(t *testing.T) {
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, llm.Model{Reasoning: false}, resolvedCompat{thinkingFormat: "openai"}, "")
	if len(params.ExtraFields()) != 0 {
		t.Fatalf("non-reasoning model wrote extras: %#v", params.ExtraFields())
	}
}

func TestApplyThinkingOpenAIDefault(t *testing.T) {
	// Default OpenAI format with an effort writes reasoning_effort.
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: true}, llm.ModelThinkingHigh)

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
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: true}, "")

	if got := extraFields(t, params)["reasoning_effort"]; got != "none" {
		t.Fatalf("reasoning_effort = %#v, want none", got)
	}
}

func TestApplyThinkingOpenAIWithoutEffortNoOffString(t *testing.T) {
	// Without a concrete off mapping, the default OpenAI branch writes nothing
	// rather than guess.
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: true}, "")

	if got := extraFields(t, params)["reasoning_effort"]; got != nil {
		t.Fatalf("reasoning_effort = %#v, want absent", got)
	}
}

func TestApplyThinkingOpenAIIgnoresEffortWhenUnsupported(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openai", supportsReasoningEffort: false}, llm.ModelThinkingHigh)

	if got := extraFields(t, params)["reasoning_effort"]; got != nil {
		t.Fatalf("reasoning_effort = %#v, want absent when unsupported", got)
	}
}

func TestApplyThinkingZAIEnabled(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "zai", supportsReasoningEffort: true}, llm.ModelThinkingHigh)

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["thinking"], map[string]any{"type": "enabled"}) {
		t.Fatalf("thinking = %#v, want enabled", extras["thinking"])
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
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "zai"}, "")

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
		effort := llm.ModelThinkingHigh
		if !enabled {
			effort = ""
		}
		params := oai.ChatCompletionNewParams{}
		applyThinking(&params, model, resolvedCompat{thinkingFormat: "qwen"}, effort)
		if got := extraFields(t, params)["enable_thinking"]; got != enabled {
			t.Fatalf("enable_thinking(%v) = %#v", enabled, got)
		}
	}
}

func TestApplyThinkingQwenChatTemplate(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "qwen-chat-template"}, llm.ModelThinkingHigh)

	got := extraFields(t, params)["chat_template_kwargs"]
	want := map[string]any{"enable_thinking": true, "preserve_thinking": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chat_template_kwargs = %#v, want %#v", got, want)
	}
}

func TestApplyThinkingDeepSeekEnabled(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "deepseek", supportsReasoningEffort: true}, llm.ModelThinkingHigh)

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["thinking"], map[string]any{"type": "enabled"}) {
		t.Fatalf("thinking = %#v, want enabled", extras["thinking"])
	}
	if got := extras["reasoning_effort"]; got != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", got)
	}
}

func TestApplyThinkingDeepSeekDisabled(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "deepseek"}, "")

	extras := extraFields(t, params)
	if !reflect.DeepEqual(extras["thinking"], map[string]any{"type": "disabled"}) {
		t.Fatalf("thinking = %#v, want disabled", extras["thinking"])
	}
}

func TestApplyThinkingDeepSeekOffMappedToNil(t *testing.T) {
	// off mapped to nil means the model has no "disable thinking" wire form, so
	// no thinking field is sent when effort is unset.
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	})
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "deepseek"}, "")

	if _, present := extraFields(t, params)["thinking"]; present {
		t.Fatalf("thinking must be absent when off mapped to nil")
	}
}

func TestApplyThinkingOpenRouterEnabled(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openrouter"}, llm.ModelThinkingMedium)

	got := extraFields(t, params)["reasoning"]
	want := map[string]any{"effort": "medium"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reasoning = %#v, want %#v", got, want)
	}
}

func TestApplyThinkingOpenRouterDisabledUsesOffEffort(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openrouter"}, "")

	got := extraFields(t, params)["reasoning"]
	want := map[string]any{"effort": "none"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reasoning = %#v, want default %#v", got, want)
	}
}

func TestApplyThinkingOpenRouterOffMappedToNil(t *testing.T) {
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	})
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "openrouter"}, "")

	if _, present := extraFields(t, params)["reasoning"]; present {
		t.Fatalf("reasoning must be absent when off mapped to nil")
	}
}

func TestApplyThinkingAntLingOnlyWhenLevelMapped(t *testing.T) {
	// ant-ling sends reasoning only when the effort level is explicitly mapped.
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingHigh: strPtr("hard"),
	})
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "ant-ling"}, llm.ModelThinkingHigh)

	got := extraFields(t, params)["reasoning"]
	want := map[string]any{"effort": "hard"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reasoning = %#v, want %#v", got, want)
	}
}

func TestApplyThinkingAntLingSkipsUnmappedLevel(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "ant-ling"}, llm.ModelThinkingHigh)

	if _, present := extraFields(t, params)["reasoning"]; present {
		t.Fatalf("ant-ling must skip unmapped levels")
	}
}

func TestApplyThinkingTogether(t *testing.T) {
	model := reasoningModel(nil)
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "together", supportsReasoningEffort: true}, llm.ModelThinkingHigh)

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
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "together"}, "")

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
	applyThinking(&on, model, resolvedCompat{thinkingFormat: "string-thinking"}, llm.ModelThinkingHigh)
	if got := extraFields(t, on)["thinking"]; got != "deep" {
		t.Fatalf("thinking on = %#v, want deep", got)
	}

	off := oai.ChatCompletionNewParams{}
	applyThinking(&off, model, resolvedCompat{thinkingFormat: "string-thinking"}, "")
	if got := extraFields(t, off)["thinking"]; got != "off" {
		t.Fatalf("thinking off = %#v, want off", got)
	}
}

func TestApplyThinkingStringFormatOffMappedToNil(t *testing.T) {
	model := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	})
	params := oai.ChatCompletionNewParams{}
	applyThinking(&params, model, resolvedCompat{thinkingFormat: "string-thinking"}, "")

	if _, present := extraFields(t, params)["thinking"]; present {
		t.Fatalf("thinking must be absent when off mapped to nil")
	}
}

func TestThinkingTypeHelper(t *testing.T) {
	if got := thinkingType(true); !reflect.DeepEqual(got, map[string]any{"type": "enabled"}) {
		t.Fatalf("thinkingType(true) = %#v", got)
	}
	if got := thinkingType(false); !reflect.DeepEqual(got, map[string]any{"type": "disabled"}) {
		t.Fatalf("thinkingType(false) = %#v", got)
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
	nilOff := reasoningModel(map[llm.ModelThinkingLevel]*string{
		llm.ModelThinkingOff: nil,
	})

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
	if _, ok := offString(nilOff); ok {
		t.Fatalf("offString nil mapping must report false")
	}

	if offIsNull(withOff) {
		t.Fatalf("offIsNull mapped string must be false")
	}
	if offIsNull(withoutOff) {
		t.Fatalf("offIsNull missing must be false")
	}
	if !offIsNull(nilOff) {
		t.Fatalf("offIsNull explicit nil must be true")
	}
}
