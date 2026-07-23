package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeWorkspacePreviewServesStaticAssetsWithoutCaching(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "web", "index.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<!doctype html><title>Static page</title>"), 0o644); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/sessions/test/preview/web/index.html", nil)
	response := httptest.NewRecorder()
	if err := serveWorkspacePreview(response, request, root, "web/index.html"); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Static page") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestServeWorkspacePreviewRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-preview.html")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/sessions/test/preview/outside", nil)
	response := httptest.NewRecorder()
	if err := serveWorkspacePreview(response, request, root, outside); err == nil {
		t.Fatal("serveWorkspacePreview accepted a path outside the workspace")
	}
}
