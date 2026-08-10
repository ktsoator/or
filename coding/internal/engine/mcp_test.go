package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestNewSurfacesMCPConfigurationDiagnostics(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{"broken":{"command":"example","unknown":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := New(t.Context(), Options{
		Model:         llm.Model{Provider: "test", ID: "model"},
		Cwd:           workspace,
		MCPConfigPath: configPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, ok := session.toolByName["mcp_status"]; !ok {
		t.Fatal("mcp_status was not added for an invalid configuration")
	}
}

func TestNewIgnoresMissingMCPConfiguration(t *testing.T) {
	session, err := New(t.Context(), Options{
		Model:         llm.Model{Provider: "test", ID: "model"},
		Cwd:           t.TempDir(),
		MCPConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, ok := session.toolByName["mcp_status"]; ok {
		t.Fatal("mcp_status was added without an MCP configuration")
	}
}
