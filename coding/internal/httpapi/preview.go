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
	r.GET("/sessions/:sessionID/previews/:grantID/*path", s.handleWorkspacePreview)
	r.HEAD("/sessions/:sessionID/previews/:grantID/*path", s.handleWorkspacePreview)
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
	grant, ok := s.transports.previews.resolve(c.Param("sessionID"), c.Param("grantID"))
	if !ok {
		http.NotFound(c.Writer, c.Request)
		return
	}
	path := strings.TrimPrefix(c.Param("path"), "/")
	if err := serveWorkspacePreview(c.Writer, c.Request, grant, path); err != nil {
		http.NotFound(c.Writer, c.Request)
	}
}

func serveWorkspacePreview(
	response http.ResponseWriter,
	request *http.Request,
	grant previewGrant,
	path string,
) error {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return fmt.Errorf("workspace preview method is not allowed")
	}
	if !previewAssetAllowed(path) {
		return fmt.Errorf("workspace preview path is not allowed")
	}
	absolute, _, err := tools.ResolvePreviewAsset(grant.Root, filepath.FromSlash(path))
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
	setWorkspacePreviewHeaders(response.Header())
	http.ServeContent(response, request, filepath.Base(absolute), info.ModTime(), file)
	if request.Context().Err() != nil {
		return fmt.Errorf("serve preview: %w", request.Context().Err())
	}
	return nil
}

func setWorkspacePreviewHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'none'",
		"connect-src 'self'",
		"img-src 'self' data: blob:",
		"media-src 'self' data: blob:",
		"font-src 'self' data:",
		"style-src 'self' 'unsafe-inline'",
		"script-src 'self' 'unsafe-inline'",
	}, "; "))
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), usb=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}
