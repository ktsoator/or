package main

import (
	"reflect"
	"testing"
)

func reasoningValues(values ...*string) []sourceReasoningOption {
	return []sourceReasoningOption{{Type: "effort", Values: values}}
}

func TestEffortThinkingLevelMap(t *testing.T) {
	none, low, high := "none", "low", "high"
	got := effortThinkingLevelMap(reasoningValues(&none, nil, &low, &high))
	want := map[string]*string{
		"off": stringPointer("none"), "minimal": nil, "low": stringPointer("low"),
		"medium": nil, "high": stringPointer("high"), "xhigh": nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effort map = %#v, want %#v", got, want)
	}
}

func TestEffortThinkingLevelMapWithoutNoneDisablesOff(t *testing.T) {
	low := "low"
	got := effortThinkingLevelMap(reasoningValues(&low))
	if value, present := got["off"]; !present || value != nil {
		t.Fatalf("off = %v (present %v), want explicit nil", value, present)
	}
}

func TestEffortThinkingLevelMapMapsMaxToXHigh(t *testing.T) {
	max := "max"
	got := effortThinkingLevelMap(reasoningValues(&max))
	if value := got["xhigh"]; value == nil || *value != "max" {
		t.Fatalf("xhigh = %v, want max", value)
	}
}

func TestEffortThinkingLevelMapPrefersNativeXHigh(t *testing.T) {
	xhigh, max := "xhigh", "max"
	got := effortThinkingLevelMap(reasoningValues(&xhigh, &max))
	if value := got["xhigh"]; value == nil || *value != "xhigh" {
		t.Fatalf("xhigh = %v, want xhigh", value)
	}
}

func TestEffortThinkingLevelMapIgnoresNonEffortControls(t *testing.T) {
	none := "none"
	tests := []struct {
		name    string
		options []sourceReasoningOption
	}{
		{name: "toggle", options: []sourceReasoningOption{{Type: "toggle"}}},
		{name: "budget tokens", options: []sourceReasoningOption{{Type: "budget_tokens"}}},
		{name: "default and null", options: reasoningValues(stringPointer("default"), nil)},
		{name: "effort absent", options: []sourceReasoningOption{{Type: "toggle", Values: []*string{&none}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effortThinkingLevelMap(test.options); got != nil {
				t.Fatalf("map = %#v, want nil", got)
			}
		})
	}
}

func TestApplyReasoningOptionMetadataRequiresDirectOpenAIEffort(t *testing.T) {
	none := "none"
	options := reasoningValues(&none)
	tests := []struct {
		name  string
		model model
		want  bool
	}{
		{
			name:  "standard OpenAI compatibility",
			model: model{Protocol: "openai-completions", Compat: compatibility{Kind: "openai"}},
			want:  true,
		},
		{
			name:  "explicit effort opt-out",
			model: model{Protocol: "openai-completions", Compat: compatibility{Kind: "openai", SupportsReasoningEffort: boolp(false)}},
		},
		{
			name:  "non-standard format",
			model: model{Protocol: "openai-completions", Compat: compatibility{Kind: "openai", ThinkingFormat: "deepseek"}},
		},
		{
			name:  "anthropic protocol",
			model: model{Protocol: "anthropic-messages"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applyReasoningOptionMetadata(&test.model, options)
			if got := test.model.ThinkingLevelMap != nil; got != test.want {
				t.Fatalf("metadata applied = %v, want %v", got, test.want)
			}
		})
	}
}
