package mcp

import (
	"os"
	"path/filepath"
	"strings"
)

func normalizeWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func connectionScope(config ServerConfig, workspace string) string {
	if strings.TrimSpace(config.Command) != "" || referencesWorkspace(config) {
		return workspace
	}
	return "global"
}

func referencesWorkspace(config ServerConfig) bool {
	values := []string{config.Command, config.Cwd, config.URL}
	values = append(values, config.Args...)
	for _, value := range config.Env {
		values = append(values, value)
	}
	for _, value := range config.Headers {
		values = append(values, value)
	}
	for _, value := range values {
		if strings.Contains(value, "${workspace}") {
			return true
		}
	}
	return false
}

func transportName(config ServerConfig) string {
	if strings.TrimSpace(config.Command) != "" {
		return "stdio"
	}
	if strings.TrimSpace(config.URL) != "" {
		return "streamable_http"
	}
	return ""
}
