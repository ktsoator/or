package config

import (
	"strings"
	"testing"
)

func TestDefaultsDoNotSelectModel(t *testing.T) {
	cfg := Defaults()
	if cfg.Provider != "" || cfg.Model != "" {
		t.Fatalf("model should be unconfigured on first launch: provider=%q model=%q", cfg.Provider, cfg.Model)
	}
}

func TestModelFlagsAreNotAccepted(t *testing.T) {
	for _, flag := range []string{"-provider", "-model", "-base-url", "-api-key", "-thinking"} {
		t.Run(strings.TrimPrefix(flag, "-"), func(t *testing.T) {
			if _, err := Parse([]string{flag, "value"}); err == nil {
				t.Fatalf("expected %s to be rejected", flag)
			}
		})
	}
}
