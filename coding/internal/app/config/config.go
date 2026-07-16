// Package config resolves the coding CLI's runtime configuration from flags and
// environment. It is part of the product shell.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/llm"
)

// Mode selects the product adapter used for one process.
type Mode string

const (
	ModeCLI Mode = "cli"
	ModeWeb Mode = "web"
)

// Config is the resolved settings for one coding product process.
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
	// Mode selects the terminal or browser product adapter.
	Mode Mode
	// Addr is the Web listen address when Mode is ModeWeb.
	Addr string
}

// Thinking returns the reasoning level as the typed value the agent expects.
func (c Config) Thinking() llm.ModelThinkingLevel {
	return llm.ModelThinkingLevel(c.ThinkingLevel)
}

// Defaults returns a Config seeded from environment variables, used as flag
// defaults. OR_PROVIDER, OR_MODEL, and OR_THINKING override built-in defaults.
func Defaults() Config {
	provider := envOr("OR_PROVIDER", "deepseek")
	model := envOr("OR_MODEL", "deepseek-v4-pro")
	return Config{
		Provider:      provider,
		Model:         model,
		ThinkingLevel: envOr("OR_THINKING", "medium"),
		Cwd:           ".",
		SessionFile:   "",
		Mode:          ModeCLI,
		Addr:          "localhost:8787",
	}
}

// Parse resolves configuration from environment-backed defaults and command
// line flags. Command line values take precedence over environment values.
func Parse(args []string) (Config, error) {
	cfg := Defaults()
	flags := flag.NewFlagSet("coding", flag.ContinueOnError)

	flags.StringVar(&cfg.Provider, "provider", cfg.Provider, "model provider (e.g. anthropic, openai)")
	flags.StringVar(&cfg.Model, "model", cfg.Model, "model id")
	flags.StringVar(&cfg.Cwd, "cwd", cfg.Cwd, "workspace root directory")
	flags.StringVar(&cfg.SessionFile, "session", cfg.SessionFile, "session transcript file (default .coding/session.jsonl under cwd)")
	flags.StringVar(&cfg.ThinkingLevel, "thinking", cfg.ThinkingLevel, "reasoning level: off, minimal, low, medium, high, xhigh")
	flags.StringVar(&cfg.Addr, "addr", cfg.Addr, "web UI listen address (with -web)")
	webUI := flags.Bool("web", false, "serve a browser UI instead of the terminal REPL")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if *webUI {
		cfg.Mode = ModeWeb
	}
	if err := cfg.Resolve(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// IsHelp reports whether Parse stopped after printing flag help.
func IsHelp(err error) bool { return errors.Is(err, flag.ErrHelp) }

// Resolve finalizes derived fields: it absolutizes Cwd and, when SessionFile is
// empty, defaults it to .coding/session.jsonl under the workspace.
func (c *Config) Resolve() error {
	if !validThinkingLevel(c.ThinkingLevel) {
		return fmt.Errorf("invalid thinking level %q", c.ThinkingLevel)
	}
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

func validThinkingLevel(level string) bool {
	switch llm.ModelThinkingLevel(level) {
	case llm.ModelThinkingOff,
		llm.ModelThinkingMinimal,
		llm.ModelThinkingLow,
		llm.ModelThinkingMedium,
		llm.ModelThinkingHigh,
		llm.ModelThinkingXHigh:
		return true
	default:
		return false
	}
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
