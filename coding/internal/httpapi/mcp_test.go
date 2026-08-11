package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/mcpclient"
	"github.com/ktsoator/or/coding/internal/mcpmanager"
)

func TestMCPConfigEndpointsCreateRenameListAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "mcp.json")
	router := mcpTestRouter(path)
	workspace := filepath.Join(t.TempDir(), "project")

	response := mcpRequest(t, router, http.MethodGet, "/api/mcp", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("empty list status = %d, body = %s", response.Code, response.Body.String())
	}
	var empty mcpListResponse
	decodeMCPResponse(t, response, &empty)
	if empty.Path != path || len(empty.Servers) != 0 {
		t.Fatalf("empty list = %+v", empty)
	}

	body := map[string]any{
		"name": "everything",
		"config": map[string]any{
			"command":    "npx",
			"args":       []string{"-y", "@modelcontextprotocol/server-everything"},
			"workspaces": []string{workspace},
		},
	}
	response = mcpRequest(t, router, http.MethodPut, "/api/mcp/servers?workspace="+workspace, body)
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
		}
	}

	body["name"] = "reference"
	body["previousName"] = "everything"
	response = mcpRequest(t, router, http.MethodPut, "/api/mcp/servers?workspace="+workspace, body)
	if response.Code != http.StatusOK {
		t.Fatalf("rename status = %d, body = %s", response.Code, response.Body.String())
	}

	response = mcpRequest(t, router, http.MethodGet, "/api/mcp?workspace="+workspace, nil)
	var listed mcpListResponse
	decodeMCPResponse(t, response, &listed)
	if len(listed.Servers) != 1 || listed.Servers[0].Name != "reference" ||
		listed.Servers[0].InScope == nil || !*listed.Servers[0].InScope {
		t.Fatalf("servers = %+v", listed.Servers)
	}

	response = mcpRequest(t, router, http.MethodDelete, "/api/mcp/servers/reference", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	response = mcpRequest(t, router, http.MethodGet, "/api/mcp", nil)
	decodeMCPResponse(t, response, &listed)
	if len(listed.Servers) != 0 {
		t.Fatalf("servers after delete = %+v", listed.Servers)
	}
}

func TestMCPConfigEndpointsRejectInvalidAndDuplicateServers(t *testing.T) {
	router := mcpTestRouter(filepath.Join(t.TempDir(), "mcp.json"))
	tests := []struct {
		name string
		body map[string]any
		want int
	}{
		{name: "invalid name", body: map[string]any{"name": "bad/name", "config": map[string]any{"command": "x"}}, want: http.StatusBadRequest},
		{name: "two transports", body: map[string]any{"name": "both", "config": map[string]any{"command": "x", "url": "https://example.com"}}, want: http.StatusBadRequest},
		{name: "no transport", body: map[string]any{"name": "none", "config": map[string]any{}}, want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := mcpRequest(t, router, http.MethodPut, "/api/mcp/servers", test.body)
			if response.Code != test.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}

	valid := map[string]any{"name": "one", "config": map[string]any{"command": "x"}}
	if response := mcpRequest(t, router, http.MethodPut, "/api/mcp/servers", valid); response.Code != http.StatusOK {
		t.Fatalf("initial create status = %d", response.Code)
	}
	if response := mcpRequest(t, router, http.MethodPut, "/api/mcp/servers", valid); response.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMCPConfigEndpointSurfacesMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"broken":{"command":"x","typo":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	response := mcpRequest(t, mcpTestRouter(path), http.MethodGet, "/api/mcp", nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMCPSaveAndDeleteReloadManagerConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	manager := mcpmanager.New(t.Context(), path)
	t.Cleanup(manager.Close)
	router := gin.New()
	(&Server{mcp: manager, mcpConfigPath: path}).mountMCP(router.Group("/api"))

	saved := mcpRequest(t, router, http.MethodPut, "/api/mcp/servers", mcpServerSaveRequest{
		Name: "disabled-test",
		Config: mcpclient.ServerConfig{
			Disabled: true,
			Command:  "example",
		},
	})
	if saved.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saved.Code, saved.Body.String())
	}
	lease := manager.Acquire(t.Context(), t.TempDir())
	statuses := lease.Statuses()
	lease.Close()
	if len(statuses) != 1 || statuses[0].Name != "disabled-test" || statuses[0].State != mcpclient.StateDisabled {
		t.Fatalf("statuses after save = %#v", statuses)
	}

	deleted := mcpRequest(t, router, http.MethodDelete, "/api/mcp/servers/disabled-test", nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
	lease = manager.Acquire(t.Context(), t.TempDir())
	statuses = lease.Statuses()
	lease.Close()
	if len(statuses) != 0 {
		t.Fatalf("statuses after delete = %#v", statuses)
	}
}

func TestProjectMCPProbeToolsKeepsProductNamingAtHTTPBoundary(t *testing.T) {
	tools := projectMCPProbeTools("demo server", []mcpclient.ProbeTool{{
		Name:        "read.file",
		Title:       "Read file",
		Description: "Reads one file.",
	}})
	if len(tools) != 1 {
		t.Fatalf("tools = %#v", tools)
	}
	tool := tools[0]
	if tool.Name != "mcp__demo_server__read_file" || tool.Original != "read.file" || tool.Title != "Read file" {
		t.Fatalf("tool = %#v", tool)
	}
}

func mcpTestRouter(path string) *gin.Engine {
	router := gin.New()
	(&Server{mcpConfigPath: path}).mountMCP(router.Group("/api"))
	return router
}

func mcpRequest(t *testing.T, router http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeMCPResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", response.Body.String(), err)
	}
}
