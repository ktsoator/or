package web

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/app/workspace"
)

// The workspace picker browses the machine's filesystem, which is unrelated to
// sessions: it answers "what folders exist here" so the user can choose a
// project root.

type directoryEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// handleDirectories provides a local directory browser for the React workspace
// picker. The API is intentionally directory-only; files are never returned.
func (s *Server) handleDirectories(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		path = s.sessions.cfg.Cwd
	}
	cleaned, err := workspace.Validate(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	directories := make([]directoryEntry, 0)
	for _, entry := range entries {
		// Match the native folder-picker convention: internal dot-directories
		// stay out of the primary workspace browsing flow.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate := filepath.Join(cleaned, entry.Name())
		info, infoErr := os.Stat(candidate)
		if infoErr != nil || !info.IsDir() {
			continue
		}
		directories = append(directories, directoryEntry{Name: entry.Name(), Path: candidate})
	}
	sort.Slice(directories, func(i, j int) bool {
		return strings.ToLower(directories[i].Name) < strings.ToLower(directories[j].Name)
	})
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		parent = ""
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"path":        cleaned,
		"parent":      parent,
		"directories": directories,
	})
}
