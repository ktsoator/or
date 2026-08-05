package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/prompttemplate"
)

type promptTemplateDTO struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Descriptions  map[string]string `json:"descriptions,omitempty"`
	ArgumentHint  string            `json:"argumentHint"`
	ArgumentHints map[string]string `json:"argumentHints,omitempty"`
	Source        string            `json:"source"`
	Path          string            `json:"path"`
}

type promptTemplateDiagnosticDTO struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type promptTemplateDetailDTO struct {
	promptTemplateDTO
	Content string `json:"content"`
}

func promptTemplateResponse(template prompttemplate.Template) promptTemplateDTO {
	return promptTemplateDTO{
		Name:          template.Name,
		Description:   template.Description,
		Descriptions:  template.Descriptions,
		ArgumentHint:  template.ArgumentHint,
		ArgumentHints: template.ArgumentHints,
		Source:        string(template.Source),
		Path:          template.Path,
	}
}

func (s *Server) handlePromptTemplates(c *gin.Context) {
	registry, foundDiagnostics := prompttemplate.LoadFor(c.Query("workspace"))
	user := make([]promptTemplateDTO, 0)
	project := make([]promptTemplateDTO, 0)
	for _, template := range registry.List() {
		dto := promptTemplateResponse(template)
		if template.Source == prompttemplate.SourceProject {
			project = append(project, dto)
		} else {
			user = append(user, dto)
		}
	}
	diagnostics := make([]promptTemplateDiagnosticDTO, 0, len(foundDiagnostics))
	for _, diagnostic := range foundDiagnostics {
		diagnostics = append(diagnostics, promptTemplateDiagnosticDTO{
			Path: diagnostic.Path, Message: diagnostic.Message,
		})
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"user":        user,
		"project":     project,
		"diagnostics": diagnostics,
	})
}

func (s *Server) handlePromptTemplateContent(c *gin.Context) {
	registry, _ := prompttemplate.LoadFor(c.Query("workspace"))
	template, ok := registry.Lookup(c.Param("name"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "prompt template not found"})
		return
	}

	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, promptTemplateDetailDTO{
		promptTemplateDTO: promptTemplateResponse(template),
		Content:           template.Content,
	})
}

func (s *Server) mountPromptTemplates(r gin.IRouter) {
	r.GET("/prompt-templates", s.handlePromptTemplates)
	r.GET("/prompt-templates/:name", s.handlePromptTemplateContent)
}
