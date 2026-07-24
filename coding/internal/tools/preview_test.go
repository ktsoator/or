package tools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestOpenPreviewReturnsStructuredLocalURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result, err := executePreview(t, `{"url":"`+server.URL+`/app","title":"Local app"}`)
	if err != nil {
		t.Fatal(err)
	}
	details, ok := result.Details.(PreviewRequest)
	if !ok {
		t.Fatalf("Details = %#v, want PreviewRequest", result.Details)
	}
	if details.URL != server.URL+"/app" || details.Title != "Local app" {
		t.Fatalf("Details = %#v", details)
	}
}

func TestOpenPreviewReturnsStructuredWorkspaceHTMLPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "web", "index.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<!doctype html><title>Local</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := executePreviewIn(t, root, `{"url":"web/index.html","title":"Static page"}`)
	if err != nil {
		t.Fatal(err)
	}
	details, ok := result.Details.(PreviewRequest)
	if !ok {
		t.Fatalf("Details = %#v, want PreviewRequest", result.Details)
	}
	if details.Path != canonicalPath || details.RelativePath != "web/index.html" || details.URL != "" || details.Title != "Static page" {
		t.Fatalf("Details = %#v", details)
	}
}

func TestOpenPreviewAcceptsWorkspaceFileURL(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "index.html")
	if err := os.WriteFile(path, []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()

	result, err := executePreviewIn(t, root, `{"url":"`+fileURL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	details, ok := result.Details.(PreviewRequest)
	if !ok || details.Path != canonicalPath || details.RelativePath != "index.html" {
		t.Fatalf("Details = %#v", result.Details)
	}
}

func TestOpenPreviewRejectsFileOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(outside, []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := executePreviewIn(t, root, `{"url":"`+outside+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Details != nil {
		t.Fatalf("Details = %#v, want nil", result.Details)
	}
	text, ok := result.Content[0].(*llm.TextContent)
	if !ok || !strings.Contains(text.Text, "inside the workspace") {
		t.Fatalf("result = %#v", result.Content)
	}
}

func TestNormalizePreviewURLCanonicalizesWildcardListener(t *testing.T) {
	got, err := normalizePreviewURL("http://0.0.0.0:3000/app")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:3000/app" {
		t.Fatalf("URL = %q", got)
	}
}

func TestOpenPreviewRejectsStoppedLocalServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := "http://" + listener.Addr().String()
	listener.Close()

	result, err := executePreview(t, `{"url":"`+address+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Details != nil {
		t.Fatalf("Details = %#v, want nil", result.Details)
	}
	text, ok := result.Content[0].(*llm.TextContent)
	if !ok || !strings.Contains(text.Text, "local server is not reachable") {
		t.Fatalf("result = %#v", result.Content)
	}
}

func TestOpenPreviewAcceptsExternalURLWithoutProbingIt(t *testing.T) {
	const address = "https://example.invalid/search?q=coding#results"
	result, err := executePreview(t, `{"url":"`+address+`","title":"Search"}`)
	if err != nil {
		t.Fatal(err)
	}
	details, ok := result.Details.(PreviewRequest)
	if !ok || details.URL != address || details.Title != "Search" {
		t.Fatalf("Details = %#v, want external PreviewRequest", result.Details)
	}
}

func TestCheckPreviewStillRejectsExternalURL(t *testing.T) {
	_, err := CheckPreview(context.Background(), "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("CheckPreview error = %v, want localhost restriction", err)
	}
}

func executePreview(t *testing.T, input string) (agent.ToolResult, error) {
	t.Helper()
	return executePreviewIn(t, t.TempDir(), input)
}

func executePreviewIn(t *testing.T, root, input string) (agent.ToolResult, error) {
	t.Helper()
	return OpenPreview(root).Execute(
		context.Background(),
		"preview-call",
		json.RawMessage(input),
		func(agent.ToolResult) {},
	)
}
