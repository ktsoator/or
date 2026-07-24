package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/tools"
)

type pendingBrowserInspection struct {
	response chan tools.BrowserInspectionResult
}

func (b *BrowserBroker) InspectBrowser(ctx context.Context) (tools.BrowserInspectionResult, error) {
	if err := ctx.Err(); err != nil {
		return tools.BrowserInspectionResult{}, err
	}
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	response := make(chan tools.BrowserInspectionResult, 1)

	b.mu.Lock()
	b.inspections[id] = pendingBrowserInspection{response: response}
	b.mu.Unlock()
	b.broadcastInspectionRequest(id)

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case result := <-response:
		return result, nil
	case <-ctx.Done():
		if b.finishInspection(id, tools.BrowserInspectionResult{ID: id, Status: tools.BrowserInspectionCancelled}) {
			return tools.BrowserInspectionResult{}, ctx.Err()
		}
		return <-response, nil
	case <-timer.C:
		result := tools.BrowserInspectionResult{ID: id, Status: tools.BrowserInspectionTimeout}
		if b.finishInspection(id, result) {
			return result, nil
		}
		return <-response, nil
	}
}

func (b *BrowserBroker) ResolveInspection(id string, result tools.BrowserInspectionResult) bool {
	if !validBrowserInspectionStatus(result.Status) {
		return false
	}
	result.ID = id
	return b.finishInspection(id, result)
}

func (b *BrowserBroker) finishInspection(id string, result tools.BrowserInspectionResult) bool {
	b.mu.Lock()
	pending, ok := b.inspections[id]
	if ok {
		delete(b.inspections, id)
		pending.response <- result
	}
	b.mu.Unlock()
	return ok
}

func (b *BrowserBroker) broadcastInspectionRequest(id string) {
	payload, _ := json.Marshal(browserInspectionRequestEvent(id))
	b.hub.Broadcast(payload)
}

func browserInspectionRequestEvent(id string) wireEvent {
	return wireEvent{Type: wireEventBrowserInspect, ID: id}
}

func validBrowserInspectionStatus(status tools.BrowserInspectionStatus) bool {
	switch status {
	case tools.BrowserInspectionCompleted, tools.BrowserInspectionFailed,
		tools.BrowserInspectionCancelled, tools.BrowserInspectionTimeout:
		return true
	default:
		return false
	}
}

func validBrowserPageStatus(status tools.BrowserPageStatus) bool {
	switch status {
	case tools.BrowserPageReady, tools.BrowserPageNavigating, tools.BrowserPageFailed:
		return true
	default:
		return false
	}
}

func (s *Server) handleBrowserInspectionResult(c *gin.Context) {
	transport, ok := s.sessionTransport(c)
	if !ok {
		return
	}
	id := strings.TrimSpace(c.Param("commandID"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "browser inspection id is required"})
		return
	}

	var body wireBrowserInspectionResult
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid browser inspection result"})
		return
	}
	result, err := decodeBrowserInspectionResult(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !transport.browser.ResolveInspection(id, result) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":  "browser_inspection_not_found",
			"error": "browser inspection not found",
		})
		return
	}
	c.Status(http.StatusNoContent)
}

func decodeBrowserInspectionResult(body wireBrowserInspectionResult) (tools.BrowserInspectionResult, error) {
	status := tools.BrowserInspectionStatus(body.Status)
	if !validBrowserInspectionStatus(status) {
		return tools.BrowserInspectionResult{}, fmt.Errorf("browser inspection status is invalid")
	}
	pageStatus := tools.BrowserPageStatus(body.PageStatus)
	if body.PageStatus != "" && !validBrowserPageStatus(pageStatus) {
		return tools.BrowserInspectionResult{}, fmt.Errorf("browser page status is invalid")
	}
	if body.Revision < 0 {
		return tools.BrowserInspectionResult{}, fmt.Errorf("browser inspection revision is invalid")
	}
	address, err := cleanBrowserResultURL(body.URL, status == tools.BrowserInspectionCompleted)
	if err != nil {
		return tools.BrowserInspectionResult{}, err
	}
	if status == tools.BrowserInspectionCompleted && pageStatus != tools.BrowserPageReady {
		return tools.BrowserInspectionResult{}, fmt.Errorf("completed browser inspection must describe a ready page")
	}
	visibleText := strings.TrimSpace(body.VisibleText)
	truncated := body.Truncated
	runes := []rune(visibleText)
	if len(runes) > tools.MaxBrowserInspectionTextRunes {
		visibleText = string(runes[:tools.MaxBrowserInspectionTextRunes])
		truncated = true
	}
	return tools.BrowserInspectionResult{
		Status:      status,
		URL:         address,
		Title:       truncateBrowserText(strings.TrimSpace(body.Title), 512),
		PageStatus:  pageStatus,
		Revision:    body.Revision,
		VisibleText: visibleText,
		Truncated:   truncated,
		Error:       truncateBrowserText(strings.TrimSpace(body.Error), 4096),
	}, nil
}
