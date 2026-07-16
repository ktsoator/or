package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseUsesEnvironmentBackedDefaults(t *testing.T) {
	t.Setenv("OR_PROVIDER", "provider-from-env")
	t.Setenv("OR_MODEL", "model-from-env")
	t.Setenv("OR_THINKING", "high")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "provider-from-env" || cfg.Model != "model-from-env" {
		t.Fatalf("model selection = %s/%s", cfg.Provider, cfg.Model)
	}
	if cfg.ThinkingLevel != "high" {
		t.Fatalf("thinking = %q", cfg.ThinkingLevel)
	}
	if cfg.Cwd != wd {
		t.Fatalf("cwd = %q, want %q", cfg.Cwd, wd)
	}
	if cfg.SessionFile != filepath.Join(wd, ".coding", "session.jsonl") {
		t.Fatalf("session = %q", cfg.SessionFile)
	}
	if cfg.Mode != ModeCLI || cfg.Addr != "localhost:8787" {
		t.Fatalf("adapter config = mode %q, addr %q", cfg.Mode, cfg.Addr)
	}
}

func TestParseFlagsOverrideEnvironmentAndSelectWeb(t *testing.T) {
	t.Setenv("OR_PROVIDER", "provider-from-env")
	t.Setenv("OR_MODEL", "model-from-env")
	t.Setenv("OR_THINKING", "low")

	workspace := filepath.Join(t.TempDir(), "project")
	sessionFile := filepath.Join(t.TempDir(), "custom.jsonl")
	cfg, err := Parse([]string{
		"-provider", "provider-from-flag",
		"-model", "model-from-flag",
		"-thinking", "xhigh",
		"-cwd", workspace,
		"-session", sessionFile,
		"-web",
		"-addr", "127.0.0.1:9999",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cfg.Provider != "provider-from-flag" || cfg.Model != "model-from-flag" {
		t.Fatalf("model selection = %s/%s", cfg.Provider, cfg.Model)
	}
	if cfg.ThinkingLevel != "xhigh" {
		t.Fatalf("thinking = %q", cfg.ThinkingLevel)
	}
	if cfg.Cwd != workspace || cfg.SessionFile != sessionFile {
		t.Fatalf("paths = cwd %q, session %q", cfg.Cwd, cfg.SessionFile)
	}
	if cfg.Mode != ModeWeb || cfg.Addr != "127.0.0.1:9999" {
		t.Fatalf("adapter config = mode %q, addr %q", cfg.Mode, cfg.Addr)
	}
}

func TestParseRejectsInvalidThinkingLevel(t *testing.T) {
	if _, err := Parse([]string{"-thinking", "extreme"}); err == nil {
		t.Fatal("Parse accepted an invalid thinking level")
	}
}

func TestParseRejectsUnknownFlag(t *testing.T) {
	if _, err := Parse([]string{"-unknown"}); err == nil {
		t.Fatal("Parse accepted an unknown flag")
	}
}
