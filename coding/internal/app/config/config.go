// Package config resolves product-shell settings. Model routing is persisted by
// the provider settings store rather than accepted as process configuration.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktsoator/or/llm"
)

// Config is the resolved settings for one coding product process.
type Config struct {
	// Provider, Model, and ThinkingLevel are populated from persisted application
	// settings or an existing session record. They are not command-line inputs.
	Provider      string
	Model         string
	ThinkingLevel string
	// Cwd is only the initial directory-browser location. Every created session
	// carries an explicit project folder or manager-owned scratch workspace.
	Cwd string
	// DataDir stores session indexes and transcripts independently from any
	// project workspace.
	DataDir string
	// SessionFile is where one session's transcript is persisted. It is not a
	// process-wide setting: the session manager assigns it per conversation.
	SessionFile string
	// Addr is the API listen address.
	Addr string
	// FrontendOrigin is the optional browser origin allowed to call the API
	// directly when the React application is deployed separately.
	FrontendOrigin string
}

// Thinking returns the reasoning level as the typed value the agent expects.
func (c Config) Thinking() llm.ModelThinkingLevel {
	return llm.ModelThinkingLevel(c.ThinkingLevel)
}

// Defaults returns shell-only defaults. A model is intentionally not selected:
// first launch remains unconfigured until the settings UI activates one.
func Defaults() Config {
	return Config{
		ThinkingLevel:  "medium",
		Cwd:            envOr("OR_CWD", ""),
		DataDir:        envOr("OR_DATA_DIR", ""),
		Addr:           "localhost:8787",
		FrontendOrigin: envOr("OR_WEB_ORIGIN", ""),
	}
}

// Parse resolves configuration from environment-backed defaults and command
// line flags. Command line values take precedence over environment values.
func Parse(args []string) (Config, error) {
	cfg := Defaults()
	flags := flag.NewFlagSet("coding", flag.ContinueOnError)

	flags.StringVar(&cfg.Cwd, "cwd", cfg.Cwd, "initial directory-browser location")
	flags.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "coding data directory (default: ~/.or/coding)")
	flags.StringVar(&cfg.Addr, "addr", cfg.Addr, "API listen address")
	flags.StringVar(&cfg.FrontendOrigin, "web-origin", cfg.FrontendOrigin, "allowed front-end origin for cross-origin API access")

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	if err := cfg.Resolve(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// IsHelp reports whether Parse stopped after printing flag help.
func IsHelp(err error) bool { return errors.Is(err, flag.ErrHelp) }

// Resolve finalizes derived fields. Directory browsing starts from the user's
// home directory and state lives under ~/.or/coding, so the server is not bound
// to whichever project launched it.
func (c *Config) Resolve() error {
	if !validThinkingLevel(c.ThinkingLevel) {
		return fmt.Errorf("invalid thinking level %q", c.ThinkingLevel)
	}
	if strings.TrimSpace(c.Cwd) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve default workspace: %w", err)
		}
		c.Cwd = home
	}
	abs, err := filepath.Abs(c.Cwd)
	if err != nil {
		return err
	}
	c.Cwd = abs

	if strings.TrimSpace(c.DataDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve data directory: %w", err)
		}
		c.DataDir = filepath.Join(home, ".or", "coding")
	}
	dataDir, err := filepath.Abs(c.DataDir)
	if err != nil {
		return err
	}
	c.DataDir = dataDir
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
