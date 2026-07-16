// Package config resolves the coding CLI's runtime configuration from flags and
// environment. It is part of the product shell.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/llm"
)

// Config is the resolved settings for one CLI run.
type Config struct {
	// Provider and Model select the model, e.g. "anthropic" / "claude-opus-4-5".
	Provider string
	Model    string
	// ThinkingLevel is the reasoning effort, as a raw string. Use Thinking to get
	// the typed value.
	ThinkingLevel string
	// Cwd is the workspace root.
	Cwd string
	// SessionFile is where the transcript is persisted.
	SessionFile string
}

// Thinking returns the reasoning level as the typed value the agent expects.
func (c Config) Thinking() llm.ModelThinkingLevel {
	return llm.ModelThinkingLevel(c.ThinkingLevel)
}

// Defaults returns a Config seeded from environment variables, used as flag
// defaults. OR_PROVIDER and OR_MODEL override the built-in defaults.
func Defaults() Config {
	provider := envOr("OR_PROVIDER", "deepseek")
	model := envOr("OR_MODEL", "deepseek-v4-flash")
	return Config{
		Provider:      provider,
		Model:         model,
		ThinkingLevel: envOr("OR_THINKING", "medium"),
		Cwd:           ".",
		SessionFile:   "",
	}
}

// Resolve finalizes derived fields: it absolutizes Cwd and, when SessionFile is
// empty, defaults it to .coding/session.jsonl under the workspace.
func (c *Config) Resolve() error {
	abs, err := filepath.Abs(c.Cwd)
	if err != nil {
		return err
	}
	c.Cwd = abs
	if c.SessionFile == "" {
		c.SessionFile = filepath.Join(abs, ".coding", "session.jsonl")
	}
	return nil
}

// ResolveModel resolves the configured provider and model id into an llm.Model.
// It fails, rather than panicking, when the pair is not registered.
func (c Config) ResolveModel() (llm.Model, error) {
	model, ok := llm.LookupModel(c.Provider, c.Model)
	if !ok {
		return llm.Model{}, fmt.Errorf("unknown model %q for provider %q", c.Model, c.Provider)
	}
	return model, nil
}

// envOr returns the environment value for key, or fallback when it is unset or
// empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
