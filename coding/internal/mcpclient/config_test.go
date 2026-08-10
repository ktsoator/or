package mcpclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigSortsServersAndRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{
  "version": 1,
  "mcpServers": {
    "zeta": {"command": "z"},
    "alpha": {"url": "https://example.com/mcp"}
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	servers, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].name != "alpha" || servers[1].name != "zeta" {
		t.Fatalf("servers = %#v", servers)
	}

	if err := os.WriteFile(path, []byte(`{"mcpServers":{"bad":{"command":"x","typo":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("loadConfig error = %v", err)
	}
}

func TestExpandAndWorkspaceScope(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "project")
	t.Setenv("MCP_TEST_TOKEN", "secret")
	got, err := expand("${workspace}:${env:MCP_TEST_TOKEN}", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if want := workspace + ":secret"; got != want {
		t.Fatalf("expand = %q, want %q", got, want)
	}
	if _, err := expand("${env:MCP_TEST_MISSING}", workspace); err == nil {
		t.Fatal("expand accepted a missing environment variable")
	}

	config := ServerConfig{Workspaces: []string{workspace}}
	if applies, err := config.appliesTo(workspace); err != nil || !applies {
		t.Fatalf("appliesTo same workspace = %v, %v", applies, err)
	}
	if applies, err := config.appliesTo(t.TempDir()); err != nil || applies {
		t.Fatalf("appliesTo other workspace = %v, %v", applies, err)
	}
	config.Workspaces = []string{"relative/path"}
	if _, err := config.appliesTo(workspace); err == nil {
		t.Fatal("appliesTo accepted a relative scope")
	}
}

func TestMergedEnvironmentExpandsConfiguredValues(t *testing.T) {
	t.Setenv("MCP_SOURCE", "from-parent")
	environment, err := mergedEnvironment(map[string]string{
		"MCP_COPY": "${env:MCP_SOURCE}",
		"MCP_ROOT": "${workspace}",
	}, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	for _, want := range []string{"MCP_COPY=from-parent", "MCP_ROOT=/workspace"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("environment does not contain %q:\n%s", want, joined)
		}
	}
}
