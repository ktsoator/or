package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/llm"
)

func init() {
	// Keep gin quiet and production-shaped; this is an app server, not a demo.
	gin.SetMode(gin.ReleaseMode)
}

// Hub fans one session's events out to every connected browser over SSE. Each
// client has a buffered channel; a slow client drops events rather than
// blocking the run.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// NewHub returns an empty Hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

// Broadcast sends data to every connected client, skipping any whose buffer is
// full.
func (h *Hub) Broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default: // slow client; drop rather than block the run
		}
	}
}

func (h *Hub) add() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) remove(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// Server wires the multi-session API: session discovery plus scoped history,
// SSE, prompt, confirmation, and abort endpoints. The React application is
// built and deployed independently from this service.
type Server struct {
	sessions       *SessionManager
	ctx            context.Context
	frontendOrigin string
}

// NewServer builds a Server. ctx is the base context for background runs.
func NewServer(ctx context.Context, sessions *SessionManager, frontendOrigin string) *Server {
	return &Server{
		sessions:       sessions,
		ctx:            ctx,
		frontendOrigin: frontendOrigin,
	}
}

// Handler returns the HTTP handler for the coding API: a gin engine serving the
// /api routes, wrapped in the cross-origin gate for a separately deployed
// front-end.
func (s *Server) Handler() http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api")
	api.GET("/models", s.handleModels)
	api.GET("/sessions", s.handleSessions)
	api.POST("/sessions", s.handleCreateSession)
	session := api.Group("/sessions/:sessionID")
	session.GET("/history", s.handleHistory)
	session.GET("/events", s.handleEvents)
	session.DELETE("", s.handleDeleteSession)
	session.PATCH("/settings", s.handleSessionSettings)
	session.POST("/prompt", s.handlePrompt)
	session.POST("/steer", s.handleSteer)
	session.POST("/follow-up", s.handleFollowUp)
	session.DELETE("/queue/:messageID", s.handleRemoveQueuedMessage)
	session.POST("/confirm", s.handleConfirm)
	session.POST("/abort", s.handleAbort)

	return allowFrontendOrigin(r, s.frontendOrigin)
}

type modelOption struct {
	Provider       string                   `json:"provider"`
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	ThinkingLevels []llm.ModelThinkingLevel `json:"thinkingLevels"`
	SupportsImages bool                     `json:"supportsImages"`
}

func (s *Server) handleModels(c *gin.Context) {
	models := make([]modelOption, 0)
	for _, provider := range llm.GetProviders() {
		if !s.providerAvailable(provider) {
			continue
		}
		for _, model := range llm.GetRunnableModels(provider) {
			name := model.Name
			if name == "" {
				name = model.ID
			}
			models = append(models, modelOption{
				Provider:       model.Provider,
				ID:             model.ID,
				Name:           name,
				ThinkingLevels: llm.SupportedThinkingLevels(model),
				SupportsImages: slices.Contains(model.Input, llm.Image),
			})
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func (s *Server) providerAvailable(provider string) bool {
	if s.sessions.UsesProvider(provider) {
		return true
	}
	status, ok := llm.DefaultProviderRegistry().AuthStatus(provider, nil)
	return ok && status.Configured
}

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
		Title string `json:"title"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
	}
	summary, err := s.sessions.Create(body.Title)
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
		"queue":   runtime.pendingEvents(),
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
		if err := runtime.session.Prompt(s.ctx, body.Text, images...); err != nil {
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

const (
	maxPromptImages       = 4
	maxPromptImageBytes   = 10 << 20
	maxPromptImagesBytes  = 20 << 20
	maxPromptRequestBytes = 28 << 20
)

type promptImage struct {
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type messageRequest struct {
	ID     string        `json:"id"`
	Text   string        `json:"text"`
	Images []promptImage `json:"images"`
}

func bindMessageRequest(c *gin.Context) (messageRequest, []llm.ImageContent, bool) {
	var body messageRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPromptRequestBytes)
	if err := c.ShouldBindJSON(&body); err != nil || (strings.TrimSpace(body.Text) == "" && len(body.Images) == 0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message must include text or an image"})
		return messageRequest{}, nil, false
	}
	body.Text = strings.TrimSpace(body.Text)
	images, err := decodePromptImages(body.Images)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return messageRequest{}, nil, false
	}
	return body, images, true
}

func decodePromptImages(input []promptImage) ([]llm.ImageContent, error) {
	if len(input) > maxPromptImages {
		return nil, fmt.Errorf("a prompt can include at most %d images", maxPromptImages)
	}
	allowed := map[string]bool{
		"image/gif":  true,
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}
	images := make([]llm.ImageContent, 0, len(input))
	total := 0
	for _, image := range input {
		mimeType := strings.ToLower(strings.TrimSpace(image.MIMEType))
		if !allowed[mimeType] {
			return nil, fmt.Errorf("unsupported image type %q", image.MIMEType)
		}
		decoded, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil || len(decoded) == 0 {
			return nil, errors.New("image data is not valid base64")
		}
		if len(decoded) > maxPromptImageBytes {
			return nil, fmt.Errorf("each image must be %d MB or smaller", maxPromptImageBytes>>20)
		}
		total += len(decoded)
		if total > maxPromptImagesBytes {
			return nil, fmt.Errorf("images must total %d MB or less", maxPromptImagesBytes>>20)
		}
		images = append(images, llm.ImageContent{Data: image.Data, MIMEType: mimeType})
	}
	return images, nil
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
