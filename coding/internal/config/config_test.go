package config

import (
	"strings"
	"testing"
)

func TestDefaultsReadClientOrigin(t *testing.T) {
	t.Setenv("OR_CLIENT_ORIGIN", "https://app.example.com")
	cfg := Defaults()
	if cfg.ClientOrigin != "https://app.example.com" {
		t.Fatalf("client origin = %q", cfg.ClientOrigin)
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
