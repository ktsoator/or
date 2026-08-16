package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/snapshot"
	"github.com/ktsoator/or/coding/internal/trace"
)

const (
	defaultDiagnosticTracePageLimit = 12
	maximumDiagnosticTracePageLimit = 50
)

var errInvalidDiagnosticTraceCursor = errors.New("invalid diagnostic trace cursor")

type diagnosticTraceCursor struct {
	StartedAt time.Time `json:"startedAt"`
	RunID     string    `json:"runId"`
}

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
	record, err := s.requestSnapshots.Load(c.Param("providerRequestID"))
	if errors.Is(err, snapshot.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	if errors.Is(err, snapshot.ErrInvalidID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider request ID"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read request snapshot"})
		return
	}
	if expected := strings.TrimSpace(c.Query("sessionId")); expected != "" && record.SessionID != expected {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	if expected := strings.TrimSpace(c.Query("runId")); expected != "" && record.RunID != expected {
		c.JSON(http.StatusNotFound, gin.H{"error": "request snapshot unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, record)
}

func (s *Server) handleDiagnosticTrace(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Query("sessionId"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "diagnostic session ID is required"})
		return
	}
	runID := strings.TrimSpace(c.Query("runId"))
	beforeValue := strings.TrimSpace(c.Query("before"))
	if runID != "" && beforeValue != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "diagnostic run ID and cursor are mutually exclusive"})
		return
	}
	limit, err := diagnosticTracePageLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagnostic trace limit"})
		return
	}
	var before *observability.DiagnosticRunCursor
	if beforeValue != "" {
		cursor, err := decodeDiagnosticTraceCursor(beforeValue)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid diagnostic trace cursor"})
			return
		}
		before = &observability.DiagnosticRunCursor{
			StartedAt: cursor.StartedAt,
			RunID:     cursor.RunID,
		}
	}
	runLimit := limit + 1
	if runID != "" {
		runLimit = 1
	}
	report, err := observability.ReadDiagnosticReport(
		s.observabilityLogPath,
		observability.DiagnosticQuery{
			SessionID:    sessionID,
			RunID:        runID,
			RunLimit:     runLimit,
			Before:       before,
			OrderByStart: true,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read diagnostics"})
		return
	}
	hasMore := runID == "" && len(report.Runs) > limit
	if hasMore {
		report.Runs = report.Runs[:limit]
	}
	page := trace.PageInfo{HasMore: hasMore}
	if hasMore {
		cursor, err := encodeDiagnosticTraceCursor(report.Runs[len(report.Runs)-1])
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not encode diagnostic cursor"})
			return
		}
		page.BeforeCursor = cursor
	}
	if len(report.Runs) == 0 && before != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, trace.Bundle{
			Version: trace.CurrentVersion, GeneratedAt: report.GeneratedAt,
			SessionID: sessionID, Tasks: []trace.Task{}, Page: page,
		})
		return
	}
	bundle, err := trace.Build(
		report, sessionID, runID, s.requestSnapshots,
	)
	if errors.Is(err, trace.ErrTaskNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "diagnostic task unavailable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not assemble diagnostic trace"})
		return
	}
	bundle.Page = page
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, bundle)
}

func diagnosticTracePageLimit(value string) (int, error) {
	if value == "" {
		return defaultDiagnosticTracePageLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > maximumDiagnosticTracePageLimit {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}

func encodeDiagnosticTraceCursor(run observability.DiagnosticRun) (string, error) {
	payload, err := json.Marshal(diagnosticTraceCursor{StartedAt: run.StartedAt, RunID: run.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeDiagnosticTraceCursor(value string) (diagnosticTraceCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return diagnosticTraceCursor{}, errInvalidDiagnosticTraceCursor
	}
	var cursor diagnosticTraceCursor
	if err := json.Unmarshal(payload, &cursor); err != nil ||
		cursor.StartedAt.IsZero() || strings.TrimSpace(cursor.RunID) == "" {
		return diagnosticTraceCursor{}, errInvalidDiagnosticTraceCursor
	}
	return cursor, nil
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
	r.GET("/diagnostics/trace", s.handleDiagnosticTrace)
}
