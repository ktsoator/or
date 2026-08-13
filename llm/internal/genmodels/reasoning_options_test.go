package main

import (
	"reflect"
	"testing"
)

func reasoningValues(values ...*string) []sourceReasoningOption {
	return []sourceReasoningOption{{Type: "effort", Values: values}}
}

func TestHasOnlyReasoningOption(t *testing.T) {
	tests := []struct {
		name    string
		options []sourceReasoningOption
		want    bool
	}{
		{name: "single toggle", options: []sourceReasoningOption{{Type: "toggle"}}, want: true},
		{name: "missing"},
		{name: "single effort", options: []sourceReasoningOption{{Type: "effort"}}},
		{
			name: "toggle and effort",
			options: []sourceReasoningOption{
				{Type: "toggle"},
				{Type: "effort"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasOnlyReasoningOption(test.options, "toggle"); got != test.want {
				t.Fatalf("hasOnlyReasoningOption() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEffortThinkingLevelMap(t *testing.T) {
	none, low, high := "none", "low", "high"
	got := effortThinkingLevelMap(reasoningValues(&none, nil, &low, &high))
	want := map[string]*string{
		"off": stringPointer("none"), "minimal": nil, "low": stringPointer("low"),
		"medium": nil, "high": stringPointer("high"), "xhigh": nil, "max": nil,
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

func TestEffortThinkingLevelMapPreservesMax(t *testing.T) {
	max := "max"
	got := effortThinkingLevelMap(reasoningValues(&max))
	if value := got["max"]; value == nil || *value != "max" {
		t.Fatalf("max = %v, want max", value)
	}
	if value, present := got["xhigh"]; !present || value != nil {
		t.Fatalf("xhigh = %v (present %v), want explicit nil", value, present)
	}
}

func TestEffortThinkingLevelMapPrefersNativeXHigh(t *testing.T) {
	xhigh, max := "xhigh", "max"
	got := effortThinkingLevelMap(reasoningValues(&xhigh, &max))
	if value := got["xhigh"]; value == nil || *value != "xhigh" {
		t.Fatalf("xhigh = %v, want xhigh", value)
	}
	if value := got["max"]; value == nil || *value != "max" {
		t.Fatalf("max = %v, want max", value)
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

func TestApplyVerifiedGatewayReasoningMetadataPreservesOnlyControls(t *testing.T) {
	high, max := "high", "max"
	candidate := model{Reasoning: true}
	applyVerifiedGatewayReasoningMetadata(&candidate, []sourceReasoningOption{
		{Type: "toggle"},
		{Type: "effort", Values: []*string{nil, &high, &max}},
	})

	want := explicitThinkingLevels([]string{"off", "high", "max"})
	if !reflect.DeepEqual(candidate.ThinkingLevelMap, want) {
		t.Fatalf("ThinkingLevelMap = %#v, want %#v", candidate.ThinkingLevelMap, want)
	}
	if !reflect.DeepEqual(candidate.Compat, compatibility{}) {
		t.Fatalf("gateway metadata assigned a wire dialect: %#v", candidate.Compat)
	}
}

func TestApplyReasoningOptionMetadataRequiresNamedEffort(t *testing.T) {
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
			name:  "Groq detected from provider",
			model: model{Protocol: "openai-completions", Provider: "groq"},
			want:  true,
		},
		{
			name:  "Cerebras detected from provider",
			model: model{Protocol: "openai-completions", Provider: "cerebras"},
			want:  true,
		},
		{
			name:  "OpenAI Responses",
			model: model{Protocol: "openai-responses"},
			want:  true,
		},
		{
			name:  "explicit effort opt-out",
			model: model{Protocol: "openai-completions", Compat: compatibility{Kind: "openai", SupportsReasoningEffort: boolp(false)}},
		},
		{
			name:  "xAI detected effort opt-out",
			model: model{Protocol: "openai-completions", Provider: "xai"},
		},
		{
			name:  "non-standard format",
			model: model{Protocol: "openai-completions", Compat: compatibility{Kind: "openai", ThinkingFormat: "deepseek"}},
		},
		{
			name: "adaptive Anthropic compatibility",
			model: model{Protocol: "anthropic-messages", Compat: compatibility{
				ForceAdaptiveThinking: boolp(true),
			}},
			want: true,
		},
		{
			name:  "budget Anthropic protocol",
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
