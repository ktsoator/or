package client

import (
	"strings"
	"testing"
)

func TestMergedEnvironmentExpandsConfiguredValues(t *testing.T) {
	t.Setenv("MCP_SOURCE", "from-parent")
	t.Setenv("CODING_DESKTOP_TOKEN", "desktop-secret")
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
	if strings.Contains(joined, "CODING_DESKTOP_TOKEN=") || strings.Contains(joined, "MCP_SOURCE=") {
		t.Fatalf("environment inherited an undeclared secret:\n%s", joined)
	}
}
