package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/tools"
)

type previewCheckRequest struct {
	URL string `json:"url"`
}

func (s *Server) mountPreview(r gin.IRouter) {
	r.POST("/preview/check", s.handlePreviewCheck)
	r.GET("/sessions/:sessionID/preview/*path", s.handleWorkspacePreview)
}

func (s *Server) handlePreviewCheck(c *gin.Context) {
	var body previewCheckRequest
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "preview URL is required"})
		return
	}

	normalized, err := tools.CheckPreview(c.Request.Context(), body.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": normalized})
}

func (s *Server) handleWorkspacePreview(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	path := strings.TrimPrefix(c.Param("path"), "/")
	if err := serveWorkspacePreview(c.Writer, c.Request, runtime.Session().Cwd(), path); err != nil {
		http.NotFound(c.Writer, c.Request)
	}
}

func serveWorkspacePreview(response http.ResponseWriter, request *http.Request, root, path string) error {
	absolute, _, err := tools.ResolvePreviewAsset(root, path)
	if err != nil {
		return err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(response, request, filepath.Base(absolute), info.ModTime(), file)
	if request.Context().Err() != nil {
		return fmt.Errorf("serve preview: %w", request.Context().Err())
	}
	return nil
}
