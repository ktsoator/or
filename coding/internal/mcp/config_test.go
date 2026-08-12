package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/mcp/client"
)

func TestReadConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"bad":{"command":"x","typo":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ReadConfig error = %v", err)
	}
}

func TestExpandAndWorkspaceScope(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "project")
	t.Setenv("MCP_TEST_TOKEN", "secret")
	got, err := client.Expand("${workspace}:${env:MCP_TEST_TOKEN}", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if want := workspace + ":secret"; got != want {
		t.Fatalf("expand = %q, want %q", got, want)
	}
	if _, err := client.Expand("${env:MCP_TEST_MISSING}", workspace); err == nil {
		t.Fatal("expand accepted a missing environment variable")
	}

	config := ServerConfig{Workspaces: []string{workspace}}
	if applies, err := config.AppliesTo(workspace); err != nil || !applies {
		t.Fatalf("appliesTo same workspace = %v, %v", applies, err)
	}
	if applies, err := config.AppliesTo(t.TempDir()); err != nil || applies {
		t.Fatalf("appliesTo other workspace = %v, %v", applies, err)
	}
	config.Workspaces = []string{"relative/path"}
	if _, err := config.AppliesTo(workspace); err == nil {
		t.Fatal("appliesTo accepted a relative scope")
	}
}

func TestReadAndWriteConfigRoundTripPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mcp.json")
	want := Config{MCPServers: map[string]ServerConfig{
		"example": {Command: "example", Args: []string{"--stdio"}},
	}}
	if err := WriteConfig(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	got, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != configVersion || got.MCPServers["example"].Command != "example" {
		t.Fatalf("config = %+v", got)
	}
}
