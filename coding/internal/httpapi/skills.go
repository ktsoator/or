package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/skills"
)

// skillDTO is a skill as the browser lists it. The instructions body is
// intentionally omitted; the page only shows discovery metadata.
type skillDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Dir         string `json:"dir"`
}

// skillDiagnosticDTO reports a skill that could not be loaded, so the page can
// surface malformed skills instead of silently dropping them.
type skillDiagnosticDTO struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// skillDetailDTO is a single skill including its full instructions body, served
// on demand when the browser opens one skills.
type skillDetailDTO struct {
	skillDTO
	Content string `json:"content"`
}

// handleSkills lists the skills visible to a workspace: system-level skills from
// ~/.or/skills always, and project-level skills from <workspace>/.or/skills when
// a workspace query parameter is supplied. A project skill overrides a
// system skill of the same name, so it appears once, under its effective source.
func (s *Server) handleSkills(c *gin.Context) {
	reg, diags := skills.LoadFor(c.Query("workspace"))

	user := make([]skillDTO, 0)
	project := make([]skillDTO, 0)
	for _, sk := range reg.List() {
		dto := skillDTO{Name: sk.Name, Description: sk.Description, Source: string(sk.Source), Dir: sk.Dir}
		if sk.Source == skills.SourceProject {
			project = append(project, dto)
		} else {
			user = append(user, dto)
		}
	}

	diagnostics := make([]skillDiagnosticDTO, 0, len(diags))
	for _, d := range diags {
		diagnostics = append(diagnostics, skillDiagnosticDTO{Path: d.Path, Message: d.Message})
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"user":        user,
		"project":     project,
		"diagnostics": diagnostics,
	})
}

// handleSkillContent returns one skill including its full SKILL.md body, resolved
// the same way as the listing. The name path parameter identifies the effective
// skill (a project skill overrides a system skill of the same name).
func (s *Server) handleSkillContent(c *gin.Context) {
	name := c.Param("name")
	reg, _ := skills.LoadFor(c.Query("workspace"))

	sk, ok := reg.Lookup(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "skill not found"})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, skillDetailDTO{
		skillDTO: skillDTO{
			Name:        sk.Name,
			Description: sk.Description,
			Source:      string(sk.Source),
			Dir:         sk.Dir,
		},
		Content: sk.Content,
	})
}

// mountSkills serves the skills visible to a workspace.
func (s *Server) mountSkills(r gin.IRouter) {
	r.GET("/skills", s.handleSkills)
	r.GET("/skills/:name", s.handleSkillContent)
}
