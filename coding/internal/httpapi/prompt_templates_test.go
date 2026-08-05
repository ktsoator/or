package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type promptTemplatesResponse struct {
	User        []promptTemplateDTO           `json:"user"`
	Project     []promptTemplateDTO           `json:"project"`
	Diagnostics []promptTemplateDiagnosticDTO `json:"diagnostics"`
}

func writePromptTemplateFixture(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func getPromptTemplates(t *testing.T, query string) promptTemplatesResponse {
	t.Helper()
	router := gin.New()
	router.GET("/api/prompt-templates", (&Server{}).handlePromptTemplates)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/prompt-templates"+query, nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response promptTemplatesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestHandlePromptTemplatesUsesProjectOverride(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	writePromptTemplateFixture(t, filepath.Join(home, ".or", "prompts"), "review", "---\ndescription: User review\n---\nuser")
	writePromptTemplateFixture(t, filepath.Join(home, ".or", "prompts"), "commit", "Commit changes")
	writePromptTemplateFixture(t, filepath.Join(workspace, ".or", "prompts"), "review", "---\ndescription: 项目审查\ndescription-en: Project review\ndescription-zh-CN: 项目审查\nargument-hint: '[关注点]'\nargument-hint-en: '[focus]'\nargument-hint-zh-CN: '[关注点]'\n---\nproject")

	response := getPromptTemplates(t, "?workspace="+url.QueryEscape(workspace))
	if len(response.Project) != 1 || response.Project[0].Name != "review" ||
		response.Project[0].Description != "项目审查" ||
		response.Project[0].Descriptions["en"] != "Project review" ||
		response.Project[0].ArgumentHints["en"] != "[focus]" {
		t.Fatalf("project templates = %+v", response.Project)
	}
	if len(response.User) != 1 || response.User[0].Name != "commit" {
		t.Fatalf("user templates = %+v", response.User)
	}
}

func TestHandlePromptTemplatesReturnsEmptyArrays(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	response := getPromptTemplates(t, "")
	if response.User == nil || response.Project == nil || response.Diagnostics == nil {
		t.Fatalf("empty response should contain arrays: %+v", response)
	}
}

func TestHandlePromptTemplateContent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	writePromptTemplateFixture(t, filepath.Join(home, ".or", "prompts"), "review", "---\ndescription: User review\n---\n# User\n")
	writePromptTemplateFixture(t, filepath.Join(workspace, ".or", "prompts"), "review", "---\ndescription: Project review\nargument-hint: '[focus]'\n---\n# Project\n\nReview $ARGUMENTS.\n")

	router := gin.New()
	router.GET("/api/prompt-templates/:name", (&Server{}).handlePromptTemplateContent)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/prompt-templates/review?workspace="+url.QueryEscape(workspace),
		nil,
	)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var detail promptTemplateDetailDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Name != "review" || detail.Source != "project" || detail.ArgumentHint != "[focus]" {
		t.Fatalf("detail = %+v", detail)
	}
	if !strings.HasPrefix(detail.Content, "# Project") || strings.Contains(detail.Content, "description:") {
		t.Fatalf("content = %q", detail.Content)
	}
}

func TestHandlePromptTemplateContentNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	router := gin.New()
	router.GET("/api/prompt-templates/:name", (&Server{}).handlePromptTemplateContent)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/prompt-templates/missing", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}
