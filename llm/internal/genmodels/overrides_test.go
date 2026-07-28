package main

import "testing"

func TestApplyOverridesForAdaptiveAnthropicModels(t *testing.T) {
	tests := []struct {
		id                  string
		adaptive            bool
		supportsTemperature bool
		xhigh               bool
		max                 bool
		offUnsupported      bool
	}{
		{id: "claude-opus-4-6", adaptive: true, supportsTemperature: true, max: true},
		{id: "claude-opus-4-8", adaptive: true, supportsTemperature: false, xhigh: true, max: true},
		{id: "claude-opus-5", adaptive: true, supportsTemperature: false, xhigh: true, max: true},
		{id: "claude-sonnet-5", adaptive: true, supportsTemperature: true, xhigh: true, max: true},
		{id: "anthropic/claude-fable-5", adaptive: true, supportsTemperature: true, xhigh: true, max: true, offUnsupported: true},
		{id: "claude-sonnet-4-5", adaptive: false, supportsTemperature: true},
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			models := []model{{ID: test.id, Protocol: "anthropic-messages"}}
			applyOverrides(models)
			compat := models[0].Compat

			if got := compat.ForceAdaptiveThinking != nil && *compat.ForceAdaptiveThinking; got != test.adaptive {
				t.Fatalf("ForceAdaptiveThinking = %v, want %v", got, test.adaptive)
			}
			gotTemperature := compat.SupportsTemperature == nil || *compat.SupportsTemperature
			if gotTemperature != test.supportsTemperature {
				t.Fatalf("supports temperature = %v, want %v", gotTemperature, test.supportsTemperature)
			}
			gotXHigh := mappedLevel(models[0].ThinkingLevelMap, "xhigh", "xhigh")
			if gotXHigh != test.xhigh {
				t.Fatalf("xhigh support = %v, want %v", gotXHigh, test.xhigh)
			}
			gotMax := mappedLevel(models[0].ThinkingLevelMap, "max", "max")
			if gotMax != test.max {
				t.Fatalf("max support = %v, want %v", gotMax, test.max)
			}
			off, hasOff := models[0].ThinkingLevelMap["off"]
			if got := hasOff && off == nil; got != test.offUnsupported {
				t.Fatalf("off unsupported = %v, want %v", got, test.offUnsupported)
			}
		})
	}
}

func mappedLevel(levels map[string]*string, level, want string) bool {
	value, ok := levels[level]
	return ok && value != nil && *value == want
}

func TestApplyOverridesDoesNotAddAnthropicCompatToOtherProtocols(t *testing.T) {
	models := []model{{ID: "claude-opus-5", Protocol: "openai-completions"}}
	applyOverrides(models)
	if models[0].Compat.Kind != "" || models[0].Compat.ForceAdaptiveThinking != nil ||
		models[0].Compat.SupportsTemperature != nil || models[0].ThinkingLevelMap != nil {
		t.Fatalf("unexpected compatibility override: %#v", models[0].Compat)
	}
}
