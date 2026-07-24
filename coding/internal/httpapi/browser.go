package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/tools"
)

const browserCommandTimeout = 30 * time.Second

// BrowserBroker delivers one session's agent navigation commands to its
// connected desktop client and waits for the first terminal acknowledgement.
type BrowserBroker struct {
	hub     *Hub
	nextID  atomic.Uint64
	timeout time.Duration

	mu          sync.Mutex
	pending     map[string]pendingBrowserCommand
	inspections map[string]pendingBrowserInspection
}

type pendingBrowserCommand struct {
	request  tools.BrowserRequest
	response chan tools.BrowserResult
}

func NewBrowserBroker(hub *Hub) *BrowserBroker {
	return &BrowserBroker{
		hub:         hub,
		timeout:     browserCommandTimeout,
		pending:     make(map[string]pendingBrowserCommand),
		inspections: make(map[string]pendingBrowserInspection),
	}
}

// OpenBrowser implements tools.BrowserController.
func (b *BrowserBroker) OpenBrowser(
	ctx context.Context,
	request tools.BrowserRequest,
) (tools.BrowserResult, error) {
	if err := ctx.Err(); err != nil {
		return tools.BrowserResult{}, err
	}
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	response := make(chan tools.BrowserResult, 1)

	b.mu.Lock()
	b.pending[id] = pendingBrowserCommand{request: request, response: response}
	b.mu.Unlock()
	b.broadcastRequest(id, request)

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case result := <-response:
		return result, nil
	case <-ctx.Done():
		if b.finish(id, tools.BrowserResult{ID: id, Status: tools.BrowserCancelled}) {
			return tools.BrowserResult{}, ctx.Err()
		}
		return <-response, nil
	case <-timer.C:
		result := tools.BrowserResult{ID: id, Status: tools.BrowserTimeout}
		if b.finish(id, result) {
			return result, nil
		}
		return <-response, nil
	}
}

// PendingEvents returns commands a history snapshot must restore after a
// renderer reconnects.
func (b *BrowserBroker) PendingEvents() []wireEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	events := make([]wireEvent, 0, len(b.pending)+len(b.inspections))
	for id, pending := range b.pending {
		events = append(events, browserRequestEvent(id, pending.request))
	}
	for id := range b.inspections {
		events = append(events, browserInspectionRequestEvent(id))
	}
	return events
}

// Resolve atomically claims one pending command. Unknown and duplicate IDs do
// not mutate broker state.
func (b *BrowserBroker) Resolve(id string, result tools.BrowserResult) bool {
	if !validBrowserResultStatus(result.Status) {
		return false
	}
	result.ID = id
	return b.finish(id, result)
}

func (b *BrowserBroker) HasPending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending) > 0 || len(b.inspections) > 0
}

// Close releases every waiter when its session transport is replaced or shut
// down. The hub is closed immediately afterwards, so no terminal event is sent.
func (b *BrowserBroker) Close() {
	b.mu.Lock()
	pending := b.pending
	b.pending = make(map[string]pendingBrowserCommand)
	inspections := b.inspections
	b.inspections = make(map[string]pendingBrowserInspection)
	b.mu.Unlock()
	for id, command := range pending {
		command.response <- tools.BrowserResult{ID: id, Status: tools.BrowserCancelled}
	}
	for id, command := range inspections {
		command.response <- tools.BrowserInspectionResult{ID: id, Status: tools.BrowserInspectionCancelled}
	}
}

func (b *BrowserBroker) finish(id string, result tools.BrowserResult) bool {
	b.mu.Lock()
	pending, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
		pending.response <- result
	}
	b.mu.Unlock()
	return ok
}

func (b *BrowserBroker) broadcastRequest(id string, request tools.BrowserRequest) {
	payload, _ := json.Marshal(browserRequestEvent(id, request))
	b.hub.Broadcast(payload)
}

func browserRequestEvent(id string, request tools.BrowserRequest) wireEvent {
	return wireEvent{
		Type:        wireEventBrowserRequest,
		ID:          id,
		Preview:     previewPayload(request.Preview),
		Disposition: wireBrowserDisposition(request.Disposition),
	}
}

func validBrowserResultStatus(status tools.BrowserResultStatus) bool {
	switch status {
	case tools.BrowserCommitted, tools.BrowserFailed, tools.BrowserCancelled, tools.BrowserTimeout:
		return true
	default:
		return false
	}
}

func (s *Server) handleBrowserResult(c *gin.Context) {
	transport, ok := s.sessionTransport(c)
	if !ok {
		return
	}
	id := strings.TrimSpace(c.Param("commandID"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "browser command id is required"})
		return
	}

	var body wireBrowserResult
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid browser result"})
		return
	}
	result, err := decodeBrowserResult(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !transport.browser.Resolve(id, result) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":  "browser_command_not_found",
			"error": "browser command not found",
		})
		return
	}
	c.Status(http.StatusNoContent)
}

func decodeBrowserResult(body wireBrowserResult) (tools.BrowserResult, error) {
	status := tools.BrowserResultStatus(body.Status)
	if !validBrowserResultStatus(status) {
		return tools.BrowserResult{}, &browserResultError{"browser result status is invalid"}
	}
	requestedURL, err := cleanBrowserResultURL(body.RequestedURL, false)
	if err != nil {
		return tools.BrowserResult{}, err
	}
	committedURL, err := cleanBrowserResultURL(body.CommittedURL, status == tools.BrowserCommitted)
	if err != nil {
		return tools.BrowserResult{}, err
	}
	title := strings.TrimSpace(body.Title)
	detail := strings.TrimSpace(body.Error)
	title = truncateBrowserText(title, 512)
	detail = truncateBrowserText(detail, 4096)
	return tools.BrowserResult{
		Status:       status,
		RequestedURL: requestedURL,
		CommittedURL: committedURL,
		Title:        title,
		Error:        detail,
	}, nil
}

func truncateBrowserText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

type browserResultError struct{ message string }

func (e *browserResultError) Error() string { return e.message }

func cleanBrowserResultURL(raw string, required bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return "", &browserResultError{"committed browser URL is required"}
		}
		return "", nil
	}
	if len(raw) > 8192 {
		return "", &browserResultError{"browser URL is too long"}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", &browserResultError{"browser URL must be an HTTP(S) URL without credentials"}
	}
	return parsed.String(), nil
}
