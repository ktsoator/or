package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/tools"
)

type browserTabsCallResult struct {
	result tools.BrowserTabsResult
	err    error
}

func TestBrowserBrokerResolvesTabsContext(t *testing.T) {
	hub := NewHub()
	events, _ := hub.add(0)
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	result := make(chan browserTabsCallResult, 1)

	go func() {
		got, err := broker.BrowserTabs(context.Background())
		result <- browserTabsCallResult{result: got, err: err}
	}()

	requested := readBrowserEvent(t, events)
	if requested.Type != wireEventBrowserTabs || requested.ID == "" {
		t.Fatalf("tabs request event = %#v", requested)
	}
	terminal := tools.BrowserTabsResult{
		Status: tools.BrowserTabsCompleted,
		OpenTabs: []tools.BrowserOpenTab{
			{
				TabID:  "stable-tab-1",
				URL:    "https://example.com/",
				Status: tools.BrowserTabReady,
			},
		},
		ControlledTabs: []tools.BrowserControlledTab{
			{
				TabID: "stable-tab-1",
				Capabilities: []tools.BrowserControlCapability{
					tools.BrowserControlRead,
					tools.BrowserControlNavigate,
				},
			},
		},
		Selected: "stable-tab-1",
	}
	if !broker.ResolveTabs(requested.ID, terminal) {
		t.Fatal("ResolveTabs returned false")
	}

	select {
	case got := <-result:
		if got.err != nil || got.result.ID != requested.ID || len(got.result.OpenTabs) != 1 {
			t.Fatalf("BrowserTabs() = %#v, %v", got.result, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("BrowserTabs did not return")
	}
}

func TestBrowserBrokerRestoresAndCancelsTabsContext(t *testing.T) {
	hub := NewHub()
	events, _ := hub.add(0)
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	result := make(chan browserTabsCallResult, 1)
	go func() {
		got, err := broker.BrowserTabs(context.Background())
		result <- browserTabsCallResult{result: got, err: err}
	}()

	requested := readBrowserEvent(t, events)
	pending := broker.PendingEvents()
	if len(pending) != 1 || pending[0].Type != wireEventBrowserTabs || pending[0].ID != requested.ID {
		t.Fatalf("pending events = %#v", pending)
	}
	broker.Close()
	select {
	case got := <-result:
		if got.err != nil || got.result.ID != requested.ID || got.result.Status != tools.BrowserTabsCancelled {
			t.Fatalf("BrowserTabs() = %#v, %v", got.result, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("closed BrowserTabs did not return")
	}
}

func TestBrowserTabsResultEndpointResolvesSessionRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	transports := NewSessionTransports()
	transport := transports.New("session-1").(*sessionTransport)
	events, _ := transport.hub.add(0)
	defer transport.hub.remove(events)
	result := make(chan browserTabsCallResult, 1)
	go func() {
		got, err := transport.BrowserTabs(context.Background())
		result <- browserTabsCallResult{result: got, err: err}
	}()
	requested := readBrowserEvent(t, events)

	router := gin.New()
	server := &Server{transports: transports}
	router.POST("/api/sessions/:sessionID/browser/tabs/:commandID/result", server.handleBrowserTabsResult)
	body := []byte(`{
		"status":"completed",
		"openTabs":[{
			"tabID":"stable-tab-1",
			"url":"https://example.com/",
			"title":"Example",
			"status":"ready"
		}],
		"controlledTabs":[{
			"tabID":"stable-tab-1",
			"capabilities":["read"]
		}],
		"selected":"stable-tab-1"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/sessions/session-1/browser/tabs/"+requested.ID+"/result",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}

	got := <-result
	if got.err != nil || len(got.result.OpenTabs) != 1 ||
		len(got.result.ControlledTabs) != 1 || got.result.Selected != "stable-tab-1" {
		t.Fatalf("result = %#v, %v", got.result, got.err)
	}
}

func TestDecodeBrowserTabsResultRejectsInvalidMetadata(t *testing.T) {
	tests := []wireBrowserTabsResult{
		{Status: "unknown"},
		{Status: wireBrowserTabsCompleted, OpenTabs: []wireBrowserOpenTab{{TabID: "", Status: wireBrowserTabReady}}},
		{Status: wireBrowserTabsCompleted, OpenTabs: []wireBrowserOpenTab{{TabID: "tab-1", Status: "other"}}},
		{Status: wireBrowserTabsCompleted, OpenTabs: []wireBrowserOpenTab{
			{TabID: "tab-1", Status: wireBrowserTabReady},
			{TabID: "tab-1", Status: wireBrowserTabReady},
		}},
		{
			Status:         wireBrowserTabsCompleted,
			OpenTabs:       []wireBrowserOpenTab{{TabID: "tab-1", Status: wireBrowserTabReady}},
			ControlledTabs: []wireBrowserControlledTab{{TabID: "missing", Capabilities: []wireBrowserControlCapability{wireBrowserControlRead}}},
		},
		{
			Status:   wireBrowserTabsCompleted,
			OpenTabs: []wireBrowserOpenTab{{TabID: "tab-1", Status: wireBrowserTabReady}},
			ControlledTabs: []wireBrowserControlledTab{
				{TabID: "tab-1", Capabilities: []wireBrowserControlCapability{wireBrowserControlRead}},
				{TabID: "tab-1", Capabilities: []wireBrowserControlCapability{wireBrowserControlNavigate}},
			},
		},
		{
			Status:         wireBrowserTabsCompleted,
			OpenTabs:       []wireBrowserOpenTab{{TabID: "tab-1", Status: wireBrowserTabReady}},
			ControlledTabs: []wireBrowserControlledTab{{TabID: "tab-1", Capabilities: []wireBrowserControlCapability{"other"}}},
		},
		{
			Status:         wireBrowserTabsCompleted,
			OpenTabs:       []wireBrowserOpenTab{{TabID: "tab-1", Status: wireBrowserTabReady}},
			ControlledTabs: []wireBrowserControlledTab{{TabID: "tab-1", Capabilities: []wireBrowserControlCapability{wireBrowserControlRead, wireBrowserControlRead}}},
		},
		{
			Status:         wireBrowserTabsCompleted,
			OpenTabs:       []wireBrowserOpenTab{{TabID: "tab-1", Status: wireBrowserTabReady}},
			ControlledTabs: []wireBrowserControlledTab{{TabID: "tab-1", Capabilities: []wireBrowserControlCapability{wireBrowserControlRead}}},
			Selected:       "tab-2",
		},
	}
	for _, body := range tests {
		if _, err := decodeBrowserTabsResult(body); err == nil {
			t.Fatalf("decodeBrowserTabsResult(%#v) succeeded", body)
		}
	}
}
