package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/requestsnapshot"
)

func (s *Server) handleDiagnosticRuns(c *gin.Context) {
	limit, err := diagnosticLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagnostics limit"})
		return
	}
	report, err := observability.ReadDiagnosticReport(
		s.observabilityLogPath,
		observability.DiagnosticQuery{
			SessionID: strings.TrimSpace(c.Query("sessionId")),
			RunLimit:  limit,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read diagnostics"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, report)
}

func (s *Server) handleDiagnosticRequest(c *gin.Context) {
	if s.requestSnapshots == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	snapshot, err := s.requestSnapshots.Load(c.Param("providerRequestID"))
	if errors.Is(err, requestsnapshot.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	if errors.Is(err, requestsnapshot.ErrInvalidID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider request ID"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read request snapshot"})
		return
	}
	if expected := strings.TrimSpace(c.Query("sessionId")); expected != "" && snapshot.SessionID != expected {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	if expected := strings.TrimSpace(c.Query("runId")); expected != "" && snapshot.RunID != expected {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, snapshot)
}

func diagnosticLimit(value string) (int, error) {
	if value == "" {
		return observability.DefaultDiagnosticRunLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}

func (s *Server) mountDiagnostics(r gin.IRouter) {
	r.GET("/diagnostics/runs", s.handleDiagnosticRuns)
	r.GET("/diagnostics/requests/:providerRequestID", s.handleDiagnosticRequest)
}
