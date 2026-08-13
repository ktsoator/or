package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/observability"
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
}
