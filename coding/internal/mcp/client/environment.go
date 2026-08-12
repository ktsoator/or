package client

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var environmentReference = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// Expand resolves workspace and environment references used by MCP transport
// configuration.
func Expand(value, workspace string) (string, error) {
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

// ExpandHome resolves a leading tilde in an MCP path.
func ExpandHome(path string) (string, error) {
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

func mergedEnvironment(configured map[string]string, workspace string) ([]string, error) {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		if key, value, found := strings.Cut(entry, "="); found {
			if inheritedMCPEnvironment[strings.ToUpper(key)] {
				values[key] = value
			}
		}
	}
	for key, value := range configured {
		expanded, err := Expand(value, workspace)
		if err != nil {
			return nil, err
		}
		values[key] = expanded
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment, nil
}

// MCP commands are executable configuration, but they should not receive every
// credential carried by the desktop sidecar. Servers opt into additional
// values through their explicit env map and ${env:NAME} references.
var inheritedMCPEnvironment = map[string]bool{
	"APPDATA": true, "COLORTERM": true, "COMSPEC": true,
	"HOME": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"LOCALAPPDATA": true, "LOGNAME": true, "PATH": true, "PATHEXT": true,
	"SHELL": true, "SYSTEMROOT": true, "TEMP": true, "TERM": true,
	"TMP": true, "TMPDIR": true, "USER": true, "USERPROFILE": true,
	"WINDIR": true, "XDG_CACHE_HOME": true, "XDG_CONFIG_HOME": true,
	"XDG_DATA_HOME": true,
}
