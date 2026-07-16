package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/ktsoator/or/coding"
)

//go:embed index.html
var indexHTML []byte

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

// Server wires the HTTP surface for one coding session: the page, the SSE event
// stream, and POST endpoints for prompts, confirmations, and aborts.
type Server struct {
	session *coding.Session
	hub     *Hub
	broker  *ConfirmBroker
	ctx     context.Context
}

// NewServer builds a Server. ctx is the base context for background runs; hub
// and broker must be the same ones wired into the session's subscription and
// permission gate.
func NewServer(ctx context.Context, session *coding.Session, hub *Hub, broker *ConfirmBroker) *Server {
	return &Server{session: session, hub: hub, broker: broker, ctx: ctx}
}

// Handler returns the HTTP handler for the web shell.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/history", s.handleHistory)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/prompt", s.handlePrompt)
	mux.HandleFunc("/confirm", s.handleConfirm)
	mux.HandleFunc("/abort", s.handleAbort)
	return mux
}

// handleHistory returns the current displayable transcript so a newly opened
// or refreshed browser can rebuild the conversation before consuming new SSE
// events.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(ProjectHistory(s.session.History())); err != nil {
		http.Error(w, "encode history", http.StatusInternalServerError)
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

// handleEvents streams run events to one browser over Server-Sent Events until
// the request is cancelled.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.add()
	defer s.hub.remove(ch)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handlePrompt starts a run for the posted text. The run proceeds in the
// background; its output arrives on the SSE stream. A busy session or other
// start error is reported back as an error event.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	go func() {
		if err := s.session.Prompt(s.ctx, body.Text); err != nil {
			payload, _ := json.Marshal(wireEvent{Type: "error", Text: err.Error()})
			s.hub.Broadcast(payload)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

// handleConfirm resolves a pending permission request.
func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Allow bool   `json:"allow"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.broker.Resolve(body.ID, body.Allow)
	w.WriteHeader(http.StatusNoContent)
}

// handleAbort cancels the current run.
func (s *Server) handleAbort(w http.ResponseWriter, r *http.Request) {
	s.session.Abort()
	w.WriteHeader(http.StatusNoContent)
}
