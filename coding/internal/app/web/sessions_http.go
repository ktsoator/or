package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/workspace"
	"github.com/ktsoator/or/llm"
)

func (s *Server) handleSessionSettings(c *gin.Context) {
	var body struct {
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		ThinkingLevel string `json:"thinkingLevel"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session settings"})
		return
	}
	model, ok := llm.LookupModel(body.Provider, body.Model)
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is not available"})
		return
	}
	if !s.providerAvailable(model.Provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model provider is not configured"})
		return
	}
	thinking := llm.ModelThinkingLevel(body.ThinkingLevel)
	supported := false
	for _, level := range llm.SupportedThinkingLevels(model) {
		if level == thinking {
			supported = true
			break
		}
	}
	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{"error": "thinking level is not supported by this model"})
		return
	}

	summary, err := s.sessions.UpdateSettings(c.Param("sessionID"), model, thinking)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, ErrSessionActive):
		c.JSON(http.StatusConflict, gin.H{"error": "wait for the session to become idle before changing settings"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, summary)
	}
}

func (s *Server) handleRenameSession(c *gin.Context) {
	var body struct {
		CustomTitle string `json:"customTitle"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	// An empty title is meaningful: it clears the custom title so the session
	// falls back to its AI or prompt-derived name.
	title := strings.TrimSpace(body.CustomTitle)
	if utf8.RuneCountInString(title) > maxTitleRunes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("title must be %d characters or fewer", maxTitleRunes),
		})
		return
	}

	summary, err := s.sessions.Rename(c.Param("sessionID"), title)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, summary)
	}
}

func (s *Server) handleDeleteSession(c *gin.Context) {
	err := s.sessions.Delete(c.Param("sessionID"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, ErrSessionActive):
		c.JSON(http.StatusConflict, gin.H{"error": "stop or resolve the session before deleting it"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.Status(http.StatusNoContent)
	}
}

func (s *Server) handleSessions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.sessions.List())
}

func (s *Server) handleCreateSession(c *gin.Context) {
	var body struct {
		Title         string `json:"title"`
		WorkspacePath string `json:"workspacePath"`
		Scope         string `json:"scope"`
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		ThinkingLevel string `json:"thinkingLevel"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
	}
	model, ok := llm.LookupModel(body.Provider, body.Model)
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "configure a model before creating a session"})
		return
	}
	if !s.providerAvailable(model.Provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model provider is not configured"})
		return
	}
	thinking := llm.ModelThinkingLevel(body.ThinkingLevel)
	if !slices.Contains(llm.SupportedThinkingLevels(model), thinking) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "thinking level is not supported by this model"})
		return
	}
	summary, err := s.sessions.Create(body.Title, body.WorkspacePath, body.Scope, model, thinking)
	if errors.Is(err, workspace.ErrInvalid) || errors.Is(err, ErrInvalidSessionScope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, summary)
}

func (s *Server) runtime(c *gin.Context) (*sessionRuntime, bool) {
	runtime, ok := s.sessions.Get(c.Param("sessionID"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	}
	return runtime, ok
}

// handleHistory returns the current displayable transcript so a newly opened
// or refreshed browser can rebuild the conversation before consuming new SSE
// events.
func (s *Server) handleHistory(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	events := ProjectHistory(runtime.session.History())
	events = append(events, runtime.broker.PendingEvents()...)
	c.JSON(http.StatusOK, gin.H{
		"events":  events,
		"queue":   projectQueue(runtime.pendingEvents()),
		"context": projectContextUsage(runtime.session.ContextUsage()),
		"running": runtime.running.Load(),
	})
}

// handleEvents streams run events to one browser over Server-Sent Events until
// the request is cancelled.
func (s *Server) handleEvents(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := runtime.hub.add()
	defer runtime.hub.remove(ch)

	// Send a comment immediately so development and production proxies forward
	// the response headers instead of buffering an otherwise empty stream.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	w.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case data := <-ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			w.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			w.Flush()
		}
	}
}

// handlePrompt starts a run for the posted text. The run proceeds in the
// background; its output arrives on the SSE stream. A busy session or other
// start error is reported back as an error event.
func (s *Server) handlePrompt(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	body, images, ok := bindMessageRequest(c)
	if !ok {
		return
	}
	sessionID := c.Param("sessionID")
	reserved, err := s.sessions.BeginPrompt(sessionID, body.Text, len(images) > 0)
	if errors.Is(err, coding.ErrBusy) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrImagesUnsupported) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	runtime = reserved
	go func() {
		defer s.sessions.EndRun(sessionID)
		if err := runtime.session.Prompt(s.ctx, body.Text, images...); err != nil && !errors.Is(err, context.Canceled) {
			payload, _ := json.Marshal(wireEvent{Type: "error", Text: err.Error()})
			runtime.hub.Broadcast(payload)
		}
	}()
	c.Status(http.StatusAccepted)
}

func (s *Server) handleSteer(c *gin.Context) {
	s.handleQueuedMessage(c, deliverySteer)
}

func (s *Server) handleFollowUp(c *gin.Context) {
	s.handleQueuedMessage(c, deliveryFollowUp)
}

func (s *Server) handleQueuedMessage(c *gin.Context, delivery queuedDelivery) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	body, images, ok := bindMessageRequest(c)
	if !ok {
		return
	}
	if runtime.broker.HasPending() {
		c.JSON(http.StatusConflict, gin.H{"error": "resolve the pending approval before queuing a message"})
		return
	}
	if len(images) > 0 && !slices.Contains(runtime.session.Snapshot().Model.Input, llm.Image) {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrImagesUnsupported.Error()})
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		id = newSessionID()
	}
	if len(id) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message id is too long"})
		return
	}
	message := queuedMessage{
		ID:       id,
		Delivery: delivery,
		Text:     body.Text,
		Images:   images,
	}
	if !runtime.queuePending(message) {
		c.JSON(http.StatusConflict, gin.H{"error": "the session is no longer running"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"id": id})
}

func (s *Server) handleRemoveQueuedMessage(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	id := strings.TrimSpace(c.Param("messageID"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message id is required"})
		return
	}
	found, removed := runtime.removePending(id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "queued message not found"})
		return
	}
	if !removed {
		c.JSON(http.StatusConflict, gin.H{"error": "queued message is already being processed"})
		return
	}
	c.Status(http.StatusNoContent)
}

// handleConfirm resolves a pending permission request.
func (s *Server) handleConfirm(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	var body struct {
		ID    string `json:"id"`
		Allow bool   `json:"allow"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ID == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	if !runtime.broker.Resolve(body.ID, body.Allow) {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval request not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// handleAbort cancels the current run.
func (s *Server) handleAbort(c *gin.Context) {
	runtime, ok := s.runtime(c)
	if !ok {
		return
	}
	runtime.session.Abort()
	c.Status(http.StatusNoContent)
}

// mountSessions serves conversations: discovery and creation, then everything
// scoped to one session id.
func (s *Server) mountSessions(r gin.IRouter) {
	r.GET("/sessions", s.handleSessions)
	r.POST("/sessions", s.handleCreateSession)

	one := r.Group("/sessions/:sessionID")
	one.GET("/history", s.handleHistory)
	one.GET("/events", s.handleEvents)
	one.DELETE("", s.handleDeleteSession)
	one.PATCH("/settings", s.handleSessionSettings)
	one.PATCH("/title", s.handleRenameSession)
	one.POST("/prompt", s.handlePrompt)
	one.POST("/steer", s.handleSteer)
	one.POST("/follow-up", s.handleFollowUp)
	one.DELETE("/queue/:messageID", s.handleRemoveQueuedMessage)
	one.POST("/confirm", s.handleConfirm)
	one.POST("/abort", s.handleAbort)
}
