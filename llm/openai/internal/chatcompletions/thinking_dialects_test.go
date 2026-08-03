package chatcompletions

import (
	"reflect"
	"testing"

	"github.com/ktsoator/or/llm"
	oai "github.com/openai/openai-go/v3"
)

func TestApplyProviderThinkingDialects(t *testing.T) {
	for _, test := range []struct {
		name       string
		format     string
		level      llm.ModelThinkingLevel
		wantType   string
		wantEffort string
	}{
		{name: "Xiaomi disabled", format: "xiaomi", level: llm.ModelThinkingOff, wantType: "disabled"},
		{name: "Xiaomi enabled", format: "xiaomi", level: llm.ModelThinkingHigh, wantType: "enabled"},
		{name: "DeepSeek disabled", format: "deepseek", level: llm.ModelThinkingOff, wantType: "disabled"},
		{
			name: "DeepSeek enabled", format: "deepseek", level: llm.ModelThinkingHigh,
			wantType: "enabled", wantEffort: "high",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			params := oai.ChatCompletionNewParams{}
			applyThinking(
				&params,
				reasoningModel(nil),
				// Xiaomi must ignore the shared effort capability while DeepSeek uses it.
				resolvedCompat{thinkingFormat: test.format, supportsReasoningEffort: true},
				explicitThinking(test.level),
			)

			extras := extraFields(t, params)
			if got := extras["thinking"]; !reflect.DeepEqual(got, map[string]any{"type": test.wantType}) {
				t.Fatalf("thinking = %#v, want %s", got, test.wantType)
			}
			if test.wantEffort == "" {
				if _, present := extras["reasoning_effort"]; present {
					t.Fatalf("reasoning_effort must be absent: %#v", extras)
				}
			} else if got := extras["reasoning_effort"]; got != test.wantEffort {
				t.Fatalf("reasoning_effort = %#v, want %s", got, test.wantEffort)
			}
		})
	}
}
