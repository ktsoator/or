package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktsoator/or/coding/internal/mcp/client"
)

const configVersion = 1

// Config is the on-disk MCP server configuration. It intentionally lives in
// Or's private data directory rather than in a workspace: opening an untrusted
// repository must never be enough to start an arbitrary local process.
type Config struct {
	Version    int                     `json:"version"`
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig describes either one stdio server (Command) or one Streamable
// HTTP server (URL). Workspaces scopes a server to exact workspace roots; an
// empty list makes it available to every session.
type ServerConfig struct {
	Disabled       bool              `json:"disabled,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Workspaces     []string          `json:"workspaces,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
}

// ReadConfig loads the product-owned MCP configuration without connecting to
// any server. A missing file remains distinguishable from an empty config so
// callers can decide whether MCP should be surfaced at all.
func ReadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(data)
}

// ParseConfig decodes one strict MCP configuration document.
func ParseConfig(data []byte) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode MCP config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, fmt.Errorf("decode MCP config: multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode MCP config: %w", err)
	}
	if config.Version == 0 {
		config.Version = configVersion
	}
	if config.Version != configVersion {
		return Config{}, fmt.Errorf("unsupported MCP config version %d", config.Version)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]ServerConfig)
	}
	for name := range config.MCPServers {
		if strings.TrimSpace(name) == "" {
			return Config{}, fmt.Errorf("MCP server name is empty")
		}
	}
	return config, nil
}

// WriteConfig atomically replaces path with a private, canonical JSON file.
func WriteConfig(path string, config Config) error {
	if config.Version == 0 {
		config.Version = configVersion
	}
	if config.Version != configVersion {
		return fmt.Errorf("unsupported MCP config version %d", config.Version)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]ServerConfig)
	}
	for name := range config.MCPServers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("MCP server name is empty")
		}
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode MCP config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create MCP config directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write MCP config: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("protect MCP config: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace MCP config: %w", err)
	}
	return nil
}

// Validate checks the transport-level fields that do not require expanding
// environment references or opening a connection.
func (config ServerConfig) Validate() error {
	return config.connectionConfig().Validate()
}

// AppliesTo reports whether this server is visible to one exact workspace.
func (config ServerConfig) AppliesTo(workspace string) (bool, error) {
	if len(config.Workspaces) == 0 {
		return true, nil
	}
	want, err := filepath.Abs(workspace)
	if err != nil {
		return false, err
	}
	want = filepath.Clean(want)
	for _, configured := range config.Workspaces {
		expanded, err := client.Expand(configured, workspace)
		if err != nil {
			return false, err
		}
		expanded, err = client.ExpandHome(expanded)
		if err != nil {
			return false, err
		}
		if !filepath.IsAbs(expanded) {
			return false, fmt.Errorf("workspace scope %q is not absolute", configured)
		}
		if filepath.Clean(expanded) == want {
			return true, nil
		}
	}
	return false, nil
}

func (config ServerConfig) connectionConfig() client.Config {
	return client.Config{
		Command:        config.Command,
		Args:           append([]string(nil), config.Args...),
		Env:            cloneMap(config.Env),
		Cwd:            config.Cwd,
		URL:            config.URL,
		Headers:        cloneMap(config.Headers),
		TimeoutSeconds: config.TimeoutSeconds,
	}
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
