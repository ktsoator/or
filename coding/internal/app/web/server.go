package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ktsoator/or/coding"
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
	api.GET("/sessions", s.handleSessions)
	api.POST("/sessions", s.handleCreateSession)
	session := api.Group("/sessions/:sessionID")
	session.GET("/history", s.handleHistory)
	session.GET("/events", s.handleEvents)
	session.DELETE("", s.handleDeleteSession)
	session.POST("/prompt", s.handlePrompt)
	session.POST("/confirm", s.handleConfirm)
	session.POST("/abort", s.handleAbort)

	return allowFrontendOrigin(r, s.frontendOrigin)
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
	var body struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Text == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	sessionID := c.Param("sessionID")
	runtime, err := s.sessions.BeginPrompt(sessionID, body.Text)
	if errors.Is(err, coding.ErrBusy) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	go func() {
		defer s.sessions.EndRun(sessionID)
		if err := runtime.session.Prompt(s.ctx, body.Text); err != nil {
			payload, _ := json.Marshal(wireEvent{Type: "error", Text: err.Error()})
			runtime.hub.Broadcast(payload)
		}
	}()
	c.Status(http.StatusAccepted)
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
