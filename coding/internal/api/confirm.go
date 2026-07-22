package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ktsoator/or/coding/policy"
)

// ConfirmBroker implements policy.Confirm for the HTTP API. It pushes a
// confirm_request to the browser over SSE and blocks the calling (tool-preparation) goroutine on a channel until a
// matching POST /api/confirm arrives.
type ConfirmBroker struct {
	hub    *Hub
	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[string]pendingConfirm
}

type pendingConfirm struct {
	response chan bool
	summary  string
}

// NewConfirmBroker returns a broker that delivers confirm requests through hub.
func NewConfirmBroker(hub *Hub) *ConfirmBroker {
	return &ConfirmBroker{hub: hub, pending: make(map[string]pendingConfirm)}
}

// Confirm sends a confirm request to the browser and blocks until it is
// answered. It satisfies policy.Confirm.
func (b *ConfirmBroker) Confirm(req policy.Request) bool {
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	ch := make(chan bool, 1)
	summary := describe(req)

	b.mu.Lock()
	b.pending[id] = pendingConfirm{response: ch, summary: summary}
	b.mu.Unlock()

	payload, _ := json.Marshal(wireEvent{Type: "confirm_request", ID: id, Summary: summary})
	b.hub.Broadcast(payload)

	return <-ch
}

// PendingEvents returns a snapshot of unanswered confirmation requests. It is
// appended to /api/history so a refreshed browser can restore the active gate.
func (b *ConfirmBroker) PendingEvents() []wireEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	events := make([]wireEvent, 0, len(b.pending))
	for id, pending := range b.pending {
		events = append(events, wireEvent{
			Type:    "confirm_request",
			ID:      id,
			Summary: pending.summary,
		})
	}
	return events
}

// HasPending reports whether this session is currently waiting for a browser
// decision. It is used by the session list without exposing broker internals.
func (b *ConfirmBroker) HasPending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending) > 0
}

// Resolve answers a pending confirm request. Unknown or already-answered ids are
// ignored.
func (b *ConfirmBroker) Resolve(id string, allow bool) bool {
	b.mu.Lock()
	pending, ok := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if ok {
		payload, _ := json.Marshal(wireEvent{Type: "confirm_resolved", ID: id})
		b.hub.Broadcast(payload)
		pending.response <- allow
	}
	return ok
}

// describe renders a short, human-readable summary of a tool call for the
// confirmation prompt.
func describe(req policy.Request) string {
	switch req.Tool {
	case "bash":
		if cmd, ok := req.Args["command"].(string); ok {
			return "bash: " + firstLine(cmd)
		}
	case "edit", "write":
		if path, ok := req.Args["path"].(string); ok {
			return req.Tool + " " + path
		}
	}
	return req.Tool
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before + " …"
	}
	return s
}
