package main

import "testing"

func TestApplyOverridesForAdaptiveAnthropicModels(t *testing.T) {
	tests := []struct {
		id                  string
		adaptive            bool
		supportsTemperature bool
	}{
		{id: "claude-opus-4-6", adaptive: true, supportsTemperature: true},
		{id: "claude-opus-4-8", adaptive: true, supportsTemperature: false},
		{id: "claude-opus-5", adaptive: true, supportsTemperature: false},
		{id: "anthropic/claude-fable-5", adaptive: true, supportsTemperature: false},
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
		})
	}
}

func TestApplyOverridesDoesNotAddAnthropicCompatToOtherProtocols(t *testing.T) {
	models := []model{{ID: "claude-opus-5", Protocol: "openai-completions"}}
	applyOverrides(models)
	if models[0].Compat.Kind != "" || models[0].Compat.ForceAdaptiveThinking != nil || models[0].Compat.SupportsTemperature != nil {
		t.Fatalf("unexpected compatibility override: %#v", models[0].Compat)
	}
}
