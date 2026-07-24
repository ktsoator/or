package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServeWorkspacePreviewServesGrantAssetsWithSecurityHeaders(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "index.html", "<!doctype html><link rel=stylesheet href=app.css><script src=app.js></script>")
	writePreviewFile(t, root, "app.css", "body { color: green }")
	writePreviewFile(t, root, "app.js", "document.title = 'Static page'")
	grant := previewGrant{Root: root}

	for _, test := range []struct {
		path    string
		content string
	}{
		{path: "index.html", content: "doctype html"},
		{path: "app.css", content: "color: green"},
		{path: "app.js", content: "document.title"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/preview/"+test.path, nil)
		response := httptest.NewRecorder()
		if err := serveWorkspacePreview(response, request, grant, test.path); err != nil {
			t.Fatalf("serve %s: %v", test.path, err)
		}
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.content) {
			t.Fatalf("response for %s = %d %q", test.path, response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodHead, "/preview/index.html", nil)
	response := httptest.NewRecorder()
	if err := serveWorkspacePreview(response, request, grant, "index.html"); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("HEAD response = %d %q", response.Code, response.Body.String())
	}
	for name, want := range map[string]string{
		"Cache-Control":                "no-store",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
	} {
		if got := response.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	csp := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "connect-src 'self'", "form-action 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("Content-Security-Policy %q is missing %q", csp, directive)
		}
	}
}

func TestServeWorkspacePreviewRejectsEscapesAndSensitiveFiles(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "web")
	writePreviewFile(t, root, "index.html", "safe")
	writePreviewFile(t, workspace, "outside.txt", "outside")
	writePreviewFile(t, root, ".env", "TOKEN=secret")
	writePreviewFile(t, root, ".git/config", "secret")
	writePreviewFile(t, root, "private.pem", "secret")
	writePreviewFile(t, root, "credentials.json", "secret")
	writePreviewFile(t, root, "credentials.yaml", "secret")
	writePreviewFile(t, root, "id_ed25519.pub", "secret")
	grant := previewGrant{Root: root}

	outside := filepath.Join(t.TempDir(), "outside.js")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked.js")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}

	paths := []string{
		"../outside.txt",
		".env",
		".git/config",
		"private.pem",
		"credentials.json",
		"credentials.yaml",
		"id_ed25519.pub",
	}
	if runtime.GOOS != "windows" {
		paths = append(paths, "linked.js")
	}
	for _, path := range paths {
		request := httptest.NewRequest(http.MethodGet, "/preview/"+path, nil)
		response := httptest.NewRecorder()
		if err := serveWorkspacePreview(response, request, grant, path); err == nil {
			t.Fatalf("serveWorkspacePreview accepted %q", path)
		}
	}
}

func TestServeWorkspacePreviewRejectsNonReadMethod(t *testing.T) {
	root := t.TempDir()
	writePreviewFile(t, root, "index.html", "safe")
	request := httptest.NewRequest(http.MethodPost, "/preview/index.html", strings.NewReader("replace"))
	response := httptest.NewRecorder()
	if err := serveWorkspacePreview(response, request, previewGrant{Root: root}, "index.html"); err == nil {
		t.Fatal("serveWorkspacePreview accepted POST")
	}
}

func TestWorkspacePreviewRouteScopesGrantToItsSession(t *testing.T) {
	workspace := t.TempDir()
	entry := writePreviewFile(t, workspace, "web/index.html", "safe")
	transports := NewSessionTransports()
	preview, err := transports.previews.issue("session-1", workspace, previewRequest(entry))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{transports: transports}

	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/api/sessions/session-1/previews/" + preview.GrantID + "/index.html", want: http.StatusOK},
		{path: "/api/sessions/session-2/previews/" + preview.GrantID + "/index.html", want: http.StatusNotFound},
		{path: "/api/sessions/session-1/previews/not-a-grant/index.html", want: http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s response = %d, want %d", test.path, response.Code, test.want)
		}
	}
}

func writePreviewFile(t *testing.T, root, path, content string) string {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return absolute
}
