package web

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ktsoator/or/coding/policy"
)

// ConfirmBroker implements policy.Confirm for the web shell. Where the terminal
// reads a y/N synchronously, this pushes a confirm_request to the browser over
// SSE and blocks the calling (tool-preparation) goroutine on a channel until a
// matching POST /confirm arrives.
type ConfirmBroker struct {
	hub    *Hub
	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[string]chan bool
}

// NewConfirmBroker returns a broker that delivers confirm requests through hub.
func NewConfirmBroker(hub *Hub) *ConfirmBroker {
	return &ConfirmBroker{hub: hub, pending: make(map[string]chan bool)}
}

// Confirm sends a confirm request to the browser and blocks until it is
// answered. It satisfies policy.Confirm.
func (b *ConfirmBroker) Confirm(req policy.Request) bool {
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	ch := make(chan bool, 1)

	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()

	payload, _ := json.Marshal(wireEvent{Type: "confirm_request", ID: id, Summary: describe(req)})
	b.hub.Broadcast(payload)

	return <-ch
}

// Resolve answers a pending confirm request. Unknown or already-answered ids are
// ignored.
func (b *ConfirmBroker) Resolve(id string, allow bool) {
	b.mu.Lock()
	ch := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if ch != nil {
		ch <- allow
	}
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
