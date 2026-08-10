// Package config resolves settings for the desktop sidecar. Model routing is
// persisted by the provider settings store rather than accepted as process
// configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the resolved settings for one desktop sidecar process.
type Config struct {
	// DataDir stores session indexes and transcripts independently from any
	// project workspace.
	DataDir string
}

// Defaults returns process-level defaults.
func Defaults() Config {
	return Config{
		DataDir: envOr("OR_DATA_DIR", ""),
	}
}

// Resolve finalizes derived fields. State lives under ~/.or/coding, independent
// of whichever project the user opens.
func (c *Config) Resolve() error {
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

// envOr returns the environment value for key, or fallback when it is unset or
// empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
