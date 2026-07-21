package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleUsage(c *gin.Context) {
	since, err := usageQueryTime(c.Query("since"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage start time"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.ledger.Report(since))
}

func (s *Server) handleUsageEvents(c *gin.Context) {
	since, err := usageQueryTime(c.Query("since"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage start time"})
		return
	}
	offset, err := usageQueryInt(c.Query("offset"), 0)
	if err != nil || offset < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage offset"})
		return
	}
	limit, err := usageQueryInt(c.Query("limit"), 50)
	if err != nil || limit <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage limit"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.ledger.Events(
		strings.TrimSpace(c.Query("provider")),
		strings.TrimSpace(c.Query("model")),
		since,
		offset,
		limit,
	))
}

func usageQueryTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

func usageQueryInt(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

// mountUsage serves the token and cost ledger.
func (s *Server) mountUsage(r gin.IRouter) {
	r.GET("/usage", s.handleUsage)
	r.GET("/usage/events", s.handleUsageEvents)
}
