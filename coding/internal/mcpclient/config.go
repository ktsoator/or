package mcpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const configVersion = 1

var environmentReference = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

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

type namedServer struct {
	name   string
	config ServerConfig
}

func loadConfig(path string) ([]namedServer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode MCP config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("decode MCP config: multiple JSON values")
		}
		return nil, fmt.Errorf("decode MCP config: %w", err)
	}
	if config.Version == 0 {
		config.Version = configVersion
	}
	if config.Version != configVersion {
		return nil, fmt.Errorf("unsupported MCP config version %d", config.Version)
	}

	names := make([]string, 0, len(config.MCPServers))
	for name := range config.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]namedServer, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("MCP server name is empty")
		}
		servers = append(servers, namedServer{name: name, config: config.MCPServers[name]})
	}
	return servers, nil
}

func (config ServerConfig) validate() error {
	hasCommand := strings.TrimSpace(config.Command) != ""
	hasURL := strings.TrimSpace(config.URL) != ""
	if hasCommand == hasURL {
		return fmt.Errorf("configure exactly one of command or url")
	}
	if config.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must not be negative")
	}
	return nil
}

func (config ServerConfig) appliesTo(workspace string) (bool, error) {
	if len(config.Workspaces) == 0 {
		return true, nil
	}
	want, err := filepath.Abs(workspace)
	if err != nil {
		return false, err
	}
	want = filepath.Clean(want)
	for _, configured := range config.Workspaces {
		expanded, err := expand(configured, workspace)
		if err != nil {
			return false, err
		}
		expanded, err = expandHome(expanded)
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

func expand(value, workspace string) (string, error) {
	value = strings.ReplaceAll(value, "${workspace}", workspace)
	var missing string
	value = environmentReference.ReplaceAllStringFunc(value, func(match string) string {
		parts := environmentReference.FindStringSubmatch(match)
		if replacement, ok := os.LookupEnv(parts[1]); ok {
			return replacement
		}
		missing = parts[1]
		return ""
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %s is not set", missing)
	}
	return value, nil
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimLeft(path[1:], `/\`)), nil
}
