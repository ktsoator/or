package httpapi

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ktsoator/or/coding/internal/permission"
)

// ApprovalBroker asks the browser to decide one permission request and waits
// until it responds or the active run is cancelled.
type ApprovalBroker struct {
	hub    *Hub
	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[string]pendingApproval
}

type pendingApproval struct {
	response chan permission.ApprovalResponse
	summary  string
	reason   string
	command  string
	segments int
}

func NewApprovalBroker(hub *Hub) *ApprovalBroker {
	return &ApprovalBroker{hub: hub, pending: make(map[string]pendingApproval)}
}

// Decide implements permission.Approver.
func (b *ApprovalBroker) Decide(ctx context.Context, req permission.ApprovalRequest) (permission.ApprovalResponse, error) {
	if err := ctx.Err(); err != nil {
		return permission.ApprovalResponse{}, err
	}
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	ch := make(chan permission.ApprovalResponse, 1)
	summary := describeApproval(req)
	command := approvalCommand(req)
	segments := commandSegments(command)

	b.mu.Lock()
	b.pending[id] = pendingApproval{
		response: ch,
		summary:  summary,
		reason:   req.Reason,
		command:  command,
		segments: segments,
	}
	b.mu.Unlock()

	b.broadcast(wireEvent{
		Type:            wireEventApprovalRequest,
		ID:              id,
		Summary:         summary,
		Reason:          req.Reason,
		Command:         command,
		CommandSegments: segments,
	})

	select {
	case response := <-ch:
		return response, nil
	case <-ctx.Done():
		if b.cancel(id) {
			return permission.ApprovalResponse{}, ctx.Err()
		}
		// Resolve won the race and buffered its answer while cancellation fired.
		return <-ch, nil
	}
}

// PendingEvents returns the approvals a refreshed browser must restore.
func (b *ApprovalBroker) PendingEvents() []wireEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	events := make([]wireEvent, 0, len(b.pending))
	for id, pending := range b.pending {
		events = append(events, wireEvent{
			Type:            wireEventApprovalRequest,
			ID:              id,
			Summary:         pending.summary,
			Reason:          pending.reason,
			Command:         pending.command,
			CommandSegments: pending.segments,
		})
	}
	return events
}

func (b *ApprovalBroker) HasPending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending) > 0
}

// Resolve atomically claims and answers a pending approval.
func (b *ApprovalBroker) Resolve(id string, choice permission.ApprovalChoice) bool {
	b.mu.Lock()
	pending, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
		pending.response <- permission.ApprovalResponse{Choice: choice}
	}
	b.mu.Unlock()
	if ok {
		b.broadcast(wireEvent{Type: wireEventApprovalResolved, ID: id})
	}
	return ok
}

func (b *ApprovalBroker) cancel(id string) bool {
	b.mu.Lock()
	_, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if ok {
		b.broadcast(wireEvent{Type: wireEventApprovalCancelled, ID: id})
	}
	return ok
}

func (b *ApprovalBroker) broadcast(event wireEvent) {
	payload, _ := json.Marshal(event)
	b.hub.Broadcast(payload)
}

// approvalCommand returns the complete shell command a bash approval covers, so
// the product surface can show every line. describeApproval only ever produces a
// one-line label, and a decision must not rest on the first line of a command
// whose later lines do something else.
func approvalCommand(req permission.ApprovalRequest) string {
	if req.Request.Tool != "bash" {
		return ""
	}
	command, _ := req.Request.Args["command"].(string)
	return command
}

// commandSegments counts the separate commands a shell would run, splitting on
// newlines and the control operators ; | & and their doubled forms. Quoted and
// backslash-escaped regions are skipped, so `echo "a && b"` counts as one.
//
// This is a hint that tells the reader a command is compound, not a parse any
// policy may rely on: constructs like $(...) and eval are not interpreted, so
// the count errs high rather than low. A zero result means there is nothing to
// run at all.
func commandSegments(command string) int {
	const (
		plain = iota
		single
		double
	)
	state := plain
	segments := 0
	inSegment := false
	escaped := false

	for _, r := range command {
		if escaped {
			escaped = false
			inSegment = true
			continue
		}
		switch state {
		case single:
			if r == '\'' {
				state = plain
			}
			continue
		case double:
			switch r {
			case '\\':
				escaped = true
			case '"':
				state = plain
			}
			continue
		}

		switch r {
		case '\\':
			escaped = true
			inSegment = true
		case '\'':
			state = single
			inSegment = true
		case '"':
			state = double
			inSegment = true
		case '\n', ';', '|', '&':
			// Doubled operators (&&, ||, ;;) and blank lines separate nothing
			// extra, so only a segment with content counts.
			if inSegment {
				segments++
				inSegment = false
			}
		case ' ', '\t', '\r':
			// Leading and trailing space never starts a segment on its own.
		default:
			inSegment = true
		}
	}
	if inSegment {
		segments++
	}
	return segments
}

func describeApproval(req permission.ApprovalRequest) string {
	for _, access := range req.Request.Accesses {
		if access.Location == permission.OutsideWorkspace {
			path := access.ResolvedPath
			if path == "" {
				path = access.Path
			}
			return string(access.Action) + " outside workspace: " + path
		}
	}

	switch req.Request.Tool {
	case "bash":
		if cmd, ok := req.Request.Args["command"].(string); ok {
			return "bash: " + firstLine(cmd)
		}
	case "read", "grep", "glob", "edit", "write":
		if path, ok := req.Request.Args["path"].(string); ok && path != "" {
			return req.Request.Tool + " " + path
		}
	}
	return req.Request.Tool
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before + " …"
	}
	return s
}
