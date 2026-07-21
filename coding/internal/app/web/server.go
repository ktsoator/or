package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/providerconfig"
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
	registry       *llm.ProviderRegistry
	providers      *providerconfig.Store
	ctx            context.Context
	frontendOrigin string
}

// NewServer builds a Server. ctx is the base context for background runs.
func NewServer(
	ctx context.Context,
	sessions *SessionManager,
	registry *llm.ProviderRegistry,
	providers *providerconfig.Store,
	frontendOrigin string,
) *Server {
	return &Server{
		sessions:       sessions,
		registry:       registry,
		providers:      providers,
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
	api.GET("/providers", s.handleProviders)
	api.PUT("/model-selection", s.handleActivateModel)
	api.PUT("/providers/:providerID", s.handleSetProvider)
	api.PATCH("/providers/:providerID/active-connection", s.handleActivateProviderConnection)
	api.PATCH("/providers/:providerID/connections/:connectionID/active-key", s.handleActivateProviderKey)
	api.DELETE("/providers/:providerID", s.handleClearProvider)
	api.GET("/usage", s.handleUsage)
	api.GET("/usage/events", s.handleUsageEvents)
	api.GET("/directories", s.handleDirectories)
	api.GET("/skills", s.handleSkills)
	api.GET("/skills/:name", s.handleSkillContent)
	api.GET("/workspaces", s.handleWorkspaces)
	api.POST("/workspaces", s.handleRegisterWorkspace)
	api.DELETE("/workspaces", s.handleRemoveWorkspace)
	api.GET("/sessions", s.handleSessions)
	api.POST("/sessions", s.handleCreateSession)
	session := api.Group("/sessions/:sessionID")
	session.GET("/history", s.handleHistory)
	session.GET("/events", s.handleEvents)
	session.DELETE("", s.handleDeleteSession)
	session.PATCH("/settings", s.handleSessionSettings)
	session.PATCH("/title", s.handleRenameSession)
	session.POST("/prompt", s.handlePrompt)
	session.POST("/steer", s.handleSteer)
	session.POST("/follow-up", s.handleFollowUp)
	session.DELETE("/queue/:messageID", s.handleRemoveQueuedMessage)
	session.POST("/confirm", s.handleConfirm)
	session.POST("/abort", s.handleAbort)

	return allowFrontendOrigin(r, s.frontendOrigin)
}

func (s *Server) handleUsage(c *gin.Context) {
	since, err := usageQueryTime(c.Query("since"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage start time"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.sessions.UsageReport(since))
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
	c.JSON(http.StatusOK, s.sessions.UsageEvents(
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

type modelOption struct {
	Provider       string                   `json:"provider"`
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	ContextWindow  int64                    `json:"contextWindow"`
	ThinkingLevels []llm.ModelThinkingLevel `json:"thinkingLevels"`
	SupportsImages bool                     `json:"supportsImages"`
}

func (s *Server) handleModels(c *gin.Context) {
	includeCatalog := c.Query("scope") == "catalog"
	models := make([]modelOption, 0)
	for _, provider := range llm.GetProviders() {
		if !includeCatalog && !s.providerAvailable(provider) {
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
				ContextWindow:  model.ContextWindow,
				ThinkingLevels: llm.SupportedThinkingLevels(model),
				SupportsImages: slices.Contains(model.Input, llm.Image),
			})
		}
	}
	defaultProvider := ""
	defaultModelID := ""
	defaultThinking := llm.ModelThinkingOff
	if selection, ok := s.providers.ActiveModel(); ok && s.providerAvailable(selection.Provider) {
		if model, found := llm.LookupModel(selection.Provider, selection.Model); found {
			defaultProvider = model.Provider
			defaultModelID = model.ID
			defaultThinking = llm.ClampThinkingLevel(model, selection.ThinkingLevel)
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"models":               models,
		"defaultProvider":      defaultProvider,
		"defaultModel":         defaultModelID,
		"defaultThinkingLevel": defaultThinking,
	})
}

func (s *Server) providerAvailable(provider string) bool {
	status, ok := s.registry.AuthStatus(provider, nil)
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

func (s *Server) handleWorkspaces(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, s.sessions.ListWorkspaces())
}

func (s *Server) handleRegisterWorkspace(c *gin.Context) {
	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	workspace, err := s.sessions.RegisterWorkspace(body.Path)
	if errors.Is(err, ErrInvalidWorkspace) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, workspace)
}

func (s *Server) handleRemoveWorkspace(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace path is required"})
		return
	}
	if err := s.sessions.RemoveWorkspace(path); errors.Is(err, ErrInvalidWorkspace) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
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
	if errors.Is(err, ErrInvalidWorkspace) || errors.Is(err, ErrInvalidSessionScope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, summary)
}

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
	cleaned, err := validateWorkspacePath(path)
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
