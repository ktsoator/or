package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/workspace"
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

	summary, err := s.conversations.UpdateSettings(c.Param("sessionID"), model, thinking)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, conversation.ErrSessionActive):
		c.JSON(http.StatusConflict, gin.H{"error": "wait for the session to become idle before changing settings"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, summary)
	}
}

func (s *Server) handlePermissionMode(c *gin.Context) {
	var body struct {
		Mode permission.Mode `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || !body.Mode.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid permission mode"})
		return
	}
	summary, err := s.conversations.UpdatePermissionMode(c.Param("sessionID"), body.Mode)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, conversation.ErrSessionActive):
		c.JSON(http.StatusConflict, gin.H{"error": "wait for the session to become idle before changing permissions"})
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
	if utf8.RuneCountInString(title) > conversation.MaxTitleRunes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("title must be %d characters or fewer", conversation.MaxTitleRunes),
		})
		return
	}

	summary, err := s.conversations.Rename(c.Param("sessionID"), title)
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
	err := s.conversations.Delete(c.Param("sessionID"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, conversation.ErrSessionActive):
		c.JSON(http.StatusConflict, gin.H{"error": "stop or resolve the session before deleting it"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.Status(http.StatusNoContent)
	}
}

func (s *Server) handleSessions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.conversations.List())
}

type createSessionRequest struct {
	Title          string          `json:"title"`
	WorkspacePath  string          `json:"workspacePath"`
	Scope          string          `json:"scope"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	ThinkingLevel  string          `json:"thinkingLevel"`
	PermissionMode permission.Mode `json:"permissionMode"`
}

func bindCreateSessionRequest(c *gin.Context) (createSessionRequest, bool) {
	var body createSessionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session settings"})
		return createSessionRequest{}, false
	}
	return body, true
}

func (s *Server) handleCreateSession(c *gin.Context) {
	body, ok := bindCreateSessionRequest(c)
	if !ok {
		return
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
	mode := body.PermissionMode
	if mode == "" {
		mode = permission.ModeAsk
	}
	if !mode.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid permission mode"})
		return
	}
	summary, err := s.conversations.Create(body.Title, body.WorkspacePath, body.Scope, model, thinking, mode)
	if errors.Is(err, workspace.ErrInvalid) || errors.Is(err, conversation.ErrInvalidSessionScope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, summary)
}

func (s *Server) sessionTransport(c *gin.Context) (*sessionTransport, bool) {
	transport, ok := s.transports.get(c.Param("sessionID"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	}
	return transport, ok
}

// handleHistory returns the current displayable transcript so a newly opened
// or refreshed browser can rebuild the conversation before consuming new SSE
// events.
func (s *Server) handleHistory(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	transport, ok := s.sessionTransport(c)
	if !ok {
		return
	}
	var events []wireEvent
	var queue []wireEvent
	var contextUsage wireContextUsage
	var running bool
	var title string
	var aiTitle string
	var customTitle string
	var snapshotErr error
	eventSeq := transport.hub.snapshot(func() {
		var snapshot conversation.Snapshot
		snapshot, snapshotErr = s.conversations.Snapshot(c.Param("sessionID"))
		if snapshotErr != nil {
			return
		}
		events = ProjectHistory(snapshot.History)
		events = append(events, transport.broker.PendingEvents()...)
		queue = projectQueue(snapshot.Queue)
		contextUsage = projectContextUsage(snapshot.ContextUsage)
		running = snapshot.Running
		title = snapshot.Title
		aiTitle = snapshot.AITitle
		customTitle = snapshot.CustomTitle
	})
	if snapshotErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, wireHistoryResponse{
		Events:      events,
		Queue:       queue,
		Context:     contextUsage,
		Running:     running,
		EventSeq:    eventSeq,
		Title:       title,
		AITitle:     aiTitle,
		CustomTitle: customTitle,
	})
}

// handleEvents streams run events to one browser over Server-Sent Events until
// the request is cancelled.
func (s *Server) handleEvents(c *gin.Context) {
	transport, ok := s.sessionTransport(c)
	if !ok {
		return
	}
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	after, _ := strconv.ParseUint(strings.TrimSpace(c.Query("after")), 10, 64)
	if lastEventID, err := strconv.ParseUint(strings.TrimSpace(c.GetHeader("Last-Event-ID")), 10, 64); err == nil && lastEventID > after {
		after = lastEventID
	}
	ch, syncRequired := transport.hub.add(after)
	if syncRequired {
		data, _ := json.Marshal(wireEvent{Type: wireEventSyncRequired})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		w.Flush()
		return
	}
	defer transport.hub.remove(ch)

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
		case frame, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", frame.sequence, frame.data)
			w.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			w.Flush()
		}
	}
}

// handlePrompt starts a run for the posted text. The manager owns the
// background lifecycle; output and asynchronous failures arrive over SSE.
func (s *Server) handlePrompt(c *gin.Context) {
	body, images, ok := bindMessageRequest(c)
	if !ok {
		return
	}
	err := s.conversations.StartPrompt(c.Param("sessionID"), body.Text, images...)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, engine.ErrBusy):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, conversation.ErrImagesUnsupported):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.Status(http.StatusAccepted)
	}
}

func (s *Server) handleSteer(c *gin.Context) {
	s.handleQueuedMessage(c, conversation.DeliverySteer)
}

func (s *Server) handleFollowUp(c *gin.Context) {
	s.handleQueuedMessage(c, conversation.DeliveryFollowUp)
}

func (s *Server) handleQueuedMessage(c *gin.Context, delivery conversation.Delivery) {
	body, images, ok := bindMessageRequest(c)
	if !ok {
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		id = conversation.NewID()
	}
	if len(id) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message id is too long"})
		return
	}
	message := conversation.QueuedMessage{
		ID:       id,
		Delivery: delivery,
		Text:     body.Text,
		Images:   images,
	}
	err := s.conversations.QueueMessage(c.Param("sessionID"), message)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	case errors.Is(err, conversation.ErrApprovalPending):
		c.JSON(http.StatusConflict, gin.H{"error": "resolve the pending approval before queuing a message"})
		return
	case errors.Is(err, conversation.ErrImagesUnsupported):
		c.JSON(http.StatusBadRequest, gin.H{"error": conversation.ErrImagesUnsupported.Error()})
		return
	case errors.Is(err, conversation.ErrSessionNotRunning):
		c.JSON(http.StatusConflict, gin.H{"error": "the session is no longer running"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"id": id})
}

func (s *Server) handleRemoveQueuedMessage(c *gin.Context) {
	id := strings.TrimSpace(c.Param("messageID"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message id is required"})
		return
	}
	err := s.conversations.DequeueMessage(c.Param("sessionID"), id)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	case errors.Is(err, conversation.ErrQueuedMessageNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "queued message not found"})
		return
	case errors.Is(err, conversation.ErrQueuedMessageInFlight):
		c.JSON(http.StatusConflict, gin.H{"error": "queued message is already being processed"})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// handleApproval resolves a pending permission request.
func (s *Server) handleApproval(c *gin.Context) {
	transport, ok := s.sessionTransport(c)
	if !ok {
		return
	}
	var body struct {
		Choice permission.ApprovalChoice `json:"choice"`
	}
	id := c.Param("approvalID")
	if err := c.ShouldBindJSON(&body); err != nil || id == "" ||
		(body.Choice != permission.AllowOnce && body.Choice != permission.Reject) {
		c.Status(http.StatusBadRequest)
		return
	}
	if !transport.broker.Resolve(id, body.Choice) {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval request not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// handleAbort cancels the current run.
func (s *Server) handleAbort(c *gin.Context) {
	if err := s.conversations.Abort(c.Param("sessionID")); errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// handleCompact performs the first explicit context-maintenance operation. It
// blocks until the summary is durable so the caller gets a definitive result;
// Manager prevents prompts and settings changes in parallel.
func (s *Server) handleCompact(c *gin.Context) {
	var body struct {
		Instructions string `json:"instructions"`
	}
	if c.Request.ContentLength > 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid compaction request"})
			return
		}
	}

	result, err := s.conversations.Compact(
		c.Request.Context(),
		c.Param("sessionID"),
		strings.TrimSpace(body.Instructions),
	)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, conversation.ErrSessionActive):
		c.JSON(http.StatusConflict, gin.H{"error": "wait for the session to become idle before compacting"})
	case engine.IsNothingToCompact(err):
		c.JSON(http.StatusConflict, gin.H{
			"code":  "nothing_to_compact",
			"error": "not enough history to compact yet",
		})
	case errors.Is(err, engine.ErrBusy):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{
			"summary":          result.Summary,
			"firstKeptEntryId": result.FirstKeptEntryID,
			"tokensBefore":     result.TokensBefore,
			"tokensAfter":      result.TokensAfter,
		})
	}
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
	one.PATCH("/permission-mode", s.handlePermissionMode)
	one.PATCH("/title", s.handleRenameSession)
	one.POST("/prompt", s.handlePrompt)
	one.POST("/steer", s.handleSteer)
	one.POST("/follow-up", s.handleFollowUp)
	one.DELETE("/queue/:messageID", s.handleRemoveQueuedMessage)
	one.POST("/approvals/:approvalID", s.handleApproval)
	one.POST("/abort", s.handleAbort)
	one.POST("/compact", s.handleCompact)
}
