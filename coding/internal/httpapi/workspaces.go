package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/workspace"
)

func (s *Server) handleWorkspaces(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.workspaces.List())
}

func (s *Server) handleRegisterWorkspace(c *gin.Context) {
	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	registered, err := s.workspaces.Register(body.Path)
	if errors.Is(err, workspace.ErrInvalid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, registered)
}

func (s *Server) handleRemoveWorkspace(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace path is required"})
		return
	}
	if err := s.workspaces.Remove(path); errors.Is(err, workspace.ErrInvalid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// mountWorkspaces serves the registered project roots shown in the sidebar.
func (s *Server) mountWorkspaces(r gin.IRouter) {
	r.GET("/workspaces", s.handleWorkspaces)
	r.POST("/workspaces", s.handleRegisterWorkspace)
	r.DELETE("/workspaces", s.handleRemoveWorkspace)
}
