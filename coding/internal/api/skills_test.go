package api

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

// skillsResp mirrors the handleSkills JSON body, reusing the handler's own DTOs.
type skillsResp struct {
	User        []skillDTO           `json:"user"`
	Project     []skillDTO           `json:"project"`
	Diagnostics []skillDiagnosticDTO `json:"diagnostics"`
}

// writeWebSkill creates <root>/<name>/SKILL.md with the given frontmatter body.
func writeWebSkill(t *testing.T, root, name, frontmatterName, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + frontmatterName + "\ndescription: " + description + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// getSkills serves one GET /api/skills request against a bare handler and
// decodes the response. handleSkills uses no Server fields, so a zero-value
// Server is sufficient.
func getSkills(t *testing.T, query string) skillsResp {
	t.Helper()
	r := gin.New()
	r.GET("/api/skills", (&Server{}).handleSkills)

	req := httptest.NewRequest(http.MethodGet, "/api/skills"+query, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body skillsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestHandleSkillsUserOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeWebSkill(t, filepath.Join(home, ".or", "skills"), "frontend-design", "frontend-design", "distinctive UI design")

	resp := getSkills(t, "")

	if len(resp.User) != 1 || resp.User[0].Name != "frontend-design" {
		t.Fatalf("user = %+v, want one frontend-design", resp.User)
	}
	if resp.User[0].Source != "user" {
		t.Errorf("source = %q, want user", resp.User[0].Source)
	}
	if len(resp.Project) != 0 {
		t.Errorf("project = %+v, want empty without a workspace", resp.Project)
	}
	if len(resp.Diagnostics) != 0 {
		t.Errorf("diagnostics = %+v, want empty", resp.Diagnostics)
	}
}

func TestHandleSkillsProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)

	userDir := filepath.Join(home, ".or", "skills")
	writeWebSkill(t, userDir, "commit", "commit", "system commit skill")
	writeWebSkill(t, userDir, "frontend-design", "frontend-design", "distinctive UI design")

	projectDir := filepath.Join(workspace, ".or", "skills")
	writeWebSkill(t, projectDir, "commit", "commit", "project commit skill")

	resp := getSkills(t, "?workspace="+url.QueryEscape(workspace))

	// commit is overridden by the project copy, so it appears once, under project.
	if len(resp.Project) != 1 || resp.Project[0].Name != "commit" {
		t.Fatalf("project = %+v, want one commit", resp.Project)
	}
	if resp.Project[0].Description != "project commit skill" {
		t.Errorf("description = %q, want project override", resp.Project[0].Description)
	}
	if len(resp.User) != 1 || resp.User[0].Name != "frontend-design" {
		t.Fatalf("user = %+v, want only frontend-design (commit overridden)", resp.User)
	}
}

func TestHandleSkillsReportsDiagnostics(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Directory name does not match frontmatter name: skipped, reported.
	writeWebSkill(t, filepath.Join(home, ".or", "skills"), "commit", "not-commit", "mismatched")

	resp := getSkills(t, "")

	if len(resp.User) != 0 || len(resp.Project) != 0 {
		t.Errorf("malformed skill should be skipped: user=%+v project=%+v", resp.User, resp.Project)
	}
	if len(resp.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", resp.Diagnostics)
	}
}

func TestHandleSkillContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".or", "skills")
	if err := os.MkdirAll(filepath.Join(dir, "commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: commit\ndescription: commit skill\n---\n\n# Commit\n\nCheck the diff first.\n"
	if err := os.WriteFile(filepath.Join(dir, "commit", "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/api/skills/:name", (&Server{}).handleSkillContent)
	req := httptest.NewRequest(http.MethodGet, "/api/skills/commit", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got skillDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "commit" || got.Source != "user" {
		t.Errorf("meta = %+v, want commit/user", got)
	}
	// The body is returned frontmatter-stripped, starting at the Markdown heading.
	if !strings.HasPrefix(got.Content, "# Commit") {
		t.Errorf("content = %q, want it to start at the body heading", got.Content)
	}
	if strings.Contains(got.Content, "description:") {
		t.Errorf("content should not include frontmatter: %q", got.Content)
	}
}

func TestHandleSkillContentNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	r := gin.New()
	r.GET("/api/skills/:name", (&Server{}).handleSkillContent)
	req := httptest.NewRequest(http.MethodGet, "/api/skills/nope", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandlerRegistersSkillRoutesWithoutConflict(t *testing.T) {
	// s.Handler() panics if gin detects a route conflict between /skills and
	// /skills/:name; constructing it is the assertion.
	(&Server{}).Handler()
}

func TestHandleSkillsEmptyReturnsArrays(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	resp := getSkills(t, "")

	// Absent roots must serialize as [] (not null), so the front-end can map over
	// them without a guard.
	if resp.User == nil || resp.Project == nil || resp.Diagnostics == nil {
		t.Errorf("empty result should use empty slices, got %+v", resp)
	}
}
