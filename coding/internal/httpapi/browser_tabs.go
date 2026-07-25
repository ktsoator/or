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

const maxBrowserTabs = 64

type pendingBrowserTabs struct {
	response chan tools.BrowserTabsResult
}

func (b *BrowserBroker) BrowserTabs(ctx context.Context) (tools.BrowserTabsResult, error) {
	if err := ctx.Err(); err != nil {
		return tools.BrowserTabsResult{}, err
	}
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	response := make(chan tools.BrowserTabsResult, 1)

	b.mu.Lock()
	b.tabContexts[id] = pendingBrowserTabs{response: response}
	b.mu.Unlock()
	b.broadcastTabsRequest(id)

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	select {
	case result := <-response:
		return result, nil
	case <-ctx.Done():
		if b.finishTabs(id, tools.BrowserTabsResult{ID: id, Status: tools.BrowserTabsCancelled}) {
			return tools.BrowserTabsResult{}, ctx.Err()
		}
		return <-response, nil
	case <-timer.C:
		result := tools.BrowserTabsResult{ID: id, Status: tools.BrowserTabsTimeout}
		if b.finishTabs(id, result) {
			return result, nil
		}
		return <-response, nil
	}
}

func (b *BrowserBroker) ResolveTabs(id string, result tools.BrowserTabsResult) bool {
	if !validBrowserTabsStatus(result.Status) {
		return false
	}
	result.ID = id
	return b.finishTabs(id, result)
}

func (b *BrowserBroker) finishTabs(id string, result tools.BrowserTabsResult) bool {
	b.mu.Lock()
	pending, ok := b.tabContexts[id]
	if ok {
		delete(b.tabContexts, id)
		pending.response <- result
	}
	b.mu.Unlock()
	return ok
}

func (b *BrowserBroker) broadcastTabsRequest(id string) {
	payload, _ := json.Marshal(browserTabsRequestEvent(id))
	b.hub.Broadcast(payload)
}

func browserTabsRequestEvent(id string) wireEvent {
	return wireEvent{Type: wireEventBrowserTabs, ID: id}
}

func validBrowserTabsStatus(status tools.BrowserTabsStatus) bool {
	switch status {
	case tools.BrowserTabsCompleted, tools.BrowserTabsFailed,
		tools.BrowserTabsCancelled, tools.BrowserTabsTimeout:
		return true
	default:
		return false
	}
}

func validBrowserControlCapability(capability tools.BrowserControlCapability) bool {
	switch capability {
	case tools.BrowserControlRead, tools.BrowserControlNavigate, tools.BrowserControlInteract:
		return true
	default:
		return false
	}
}

func validBrowserTabStatus(status tools.BrowserTabStatus) bool {
	switch status {
	case tools.BrowserTabIdle, tools.BrowserTabNavigating,
		tools.BrowserTabReady, tools.BrowserTabFailed:
		return true
	default:
		return false
	}
}

func (s *Server) handleBrowserTabsResult(c *gin.Context) {
	transport, ok := s.sessionTransport(c)
	if !ok {
		return
	}
	id := strings.TrimSpace(c.Param("commandID"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "browser tabs request id is required"})
		return
	}

	var body wireBrowserTabsResult
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid browser tabs result"})
		return
	}
	result, err := decodeBrowserTabsResult(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !transport.browser.ResolveTabs(id, result) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":  "browser_tabs_not_found",
			"error": "browser tabs request not found",
		})
		return
	}
	c.Status(http.StatusNoContent)
}

func decodeBrowserTabsResult(body wireBrowserTabsResult) (tools.BrowserTabsResult, error) {
	status := tools.BrowserTabsStatus(body.Status)
	if !validBrowserTabsStatus(status) {
		return tools.BrowserTabsResult{}, fmt.Errorf("browser tabs status is invalid")
	}
	if len(body.OpenTabs) > maxBrowserTabs {
		return tools.BrowserTabsResult{}, fmt.Errorf("browser tabs result contains too many tabs")
	}
	if len(body.ControlledTabs) > maxBrowserTabs {
		return tools.BrowserTabsResult{}, fmt.Errorf("browser tabs result contains too many controlled tabs")
	}
	openTabs := make([]tools.BrowserOpenTab, 0, len(body.OpenTabs))
	openIDs := make(map[string]struct{}, len(body.OpenTabs))
	for _, tab := range body.OpenTabs {
		tabID := strings.TrimSpace(tab.TabID)
		if tabID == "" || len([]rune(tabID)) > 256 {
			return tools.BrowserTabsResult{}, fmt.Errorf("browser tab ID is invalid")
		}
		if _, exists := openIDs[tabID]; exists {
			return tools.BrowserTabsResult{}, fmt.Errorf("browser tab IDs must be unique")
		}
		openIDs[tabID] = struct{}{}
		tabStatus := tools.BrowserTabStatus(tab.Status)
		if !validBrowserTabStatus(tabStatus) {
			return tools.BrowserTabsResult{}, fmt.Errorf("browser tab status is invalid")
		}
		address, err := cleanBrowserResultURL(tab.URL, false)
		if err != nil {
			return tools.BrowserTabsResult{}, err
		}
		openTabs = append(openTabs, tools.BrowserOpenTab{
			TabID:  tabID,
			URL:    address,
			Title:  truncateBrowserText(strings.TrimSpace(tab.Title), 512),
			Status: tabStatus,
		})
	}

	controlledTabs := make([]tools.BrowserControlledTab, 0, len(body.ControlledTabs))
	controlledIDs := make(map[string]struct{}, len(body.ControlledTabs))
	for _, tab := range body.ControlledTabs {
		tabID := strings.TrimSpace(tab.TabID)
		if _, exists := openIDs[tabID]; !exists {
			return tools.BrowserTabsResult{}, fmt.Errorf("controlled browser tab is not open")
		}
		if _, exists := controlledIDs[tabID]; exists {
			return tools.BrowserTabsResult{}, fmt.Errorf("controlled browser tab IDs must be unique")
		}
		controlledIDs[tabID] = struct{}{}
		if len(tab.Capabilities) == 0 || len(tab.Capabilities) > 3 {
			return tools.BrowserTabsResult{}, fmt.Errorf("browser control capabilities are invalid")
		}
		capabilities := make([]tools.BrowserControlCapability, 0, len(tab.Capabilities))
		seenCapabilities := make(map[tools.BrowserControlCapability]struct{}, len(tab.Capabilities))
		for _, value := range tab.Capabilities {
			capability := tools.BrowserControlCapability(value)
			if !validBrowserControlCapability(capability) {
				return tools.BrowserTabsResult{}, fmt.Errorf("browser control capability is invalid")
			}
			if _, exists := seenCapabilities[capability]; exists {
				return tools.BrowserTabsResult{}, fmt.Errorf("browser control capabilities must be unique")
			}
			seenCapabilities[capability] = struct{}{}
			capabilities = append(capabilities, capability)
		}
		controlledTabs = append(controlledTabs, tools.BrowserControlledTab{
			TabID:        tabID,
			Capabilities: capabilities,
		})
	}

	selected := strings.TrimSpace(body.Selected)
	if len([]rune(selected)) > 256 {
		return tools.BrowserTabsResult{}, fmt.Errorf("selected browser tab ID is invalid")
	}
	if selected != "" {
		if _, exists := controlledIDs[selected]; !exists {
			return tools.BrowserTabsResult{}, fmt.Errorf("selected browser tab is not controlled")
		}
	}
	return tools.BrowserTabsResult{
		Status:         status,
		OpenTabs:       openTabs,
		ControlledTabs: controlledTabs,
		Selected:       selected,
		Error:          truncateBrowserText(strings.TrimSpace(body.Error), 4096),
	}, nil
}
