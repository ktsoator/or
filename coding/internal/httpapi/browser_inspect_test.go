package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/tools"
)

type browserInspectionCallResult struct {
	result tools.BrowserInspectionResult
	err    error
}

func TestBrowserBrokerResolvesFirstInspectionResult(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	result := make(chan browserInspectionCallResult, 1)

	go func() {
		got, err := broker.InspectBrowser(context.Background())
		result <- browserInspectionCallResult{result: got, err: err}
	}()

	requested := readBrowserEvent(t, events)
	if requested.Type != wireEventBrowserInspect || requested.ID == "" {
		t.Fatalf("inspection request event = %#v", requested)
	}
	pending := broker.PendingEvents()
	if len(pending) != 1 || pending[0].Type != wireEventBrowserInspect || pending[0].ID != requested.ID {
		t.Fatalf("pending events = %#v", pending)
	}

	terminal := tools.BrowserInspectionResult{
		Status:      tools.BrowserInspectionCompleted,
		URL:         "https://example.com/final",
		Title:       "Final",
		PageStatus:  tools.BrowserPageReady,
		Revision:    4,
		VisibleText: "Visible page",
	}
	if !broker.ResolveInspection(requested.ID, terminal) {
		t.Fatal("ResolveInspection returned false")
	}
	if broker.ResolveInspection(requested.ID, tools.BrowserInspectionResult{Status: tools.BrowserInspectionFailed}) {
		t.Fatal("duplicate inspection result was accepted")
	}

	select {
	case got := <-result:
		if got.err != nil || got.result.ID != requested.ID || got.result.VisibleText != "Visible page" {
			t.Fatalf("InspectBrowser() = %#v, %v", got.result, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("InspectBrowser did not return")
	}
	if broker.HasPending() {
		t.Fatal("broker still has a pending inspection")
	}
}

func TestBrowserBrokerCancelsInspectionWithRunContext(t *testing.T) {
	hub := NewHub()
	events, _ := hub.add(0)
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan browserInspectionCallResult, 1)
	go func() {
		got, err := broker.InspectBrowser(ctx)
		result <- browserInspectionCallResult{result: got, err: err}
	}()

	requested := readBrowserEvent(t, events)
	cancel()
	select {
	case got := <-result:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("InspectBrowser error = %v, want context.Canceled", got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled InspectBrowser did not return")
	}
	if broker.ResolveInspection(requested.ID, tools.BrowserInspectionResult{Status: tools.BrowserInspectionCompleted}) {
		t.Fatal("late inspection result was accepted")
	}
}

func TestBrowserBrokerInspectionTimeoutAndClose(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		hub := NewHub()
		events, _ := hub.add(0)
		defer hub.remove(events)
		broker := NewBrowserBroker(hub)
		broker.timeout = 10 * time.Millisecond
		result := make(chan browserInspectionCallResult, 1)
		go func() {
			got, err := broker.InspectBrowser(context.Background())
			result <- browserInspectionCallResult{result: got, err: err}
		}()
		requested := readBrowserEvent(t, events)

		select {
		case got := <-result:
			if got.err != nil || got.result.Status != tools.BrowserInspectionTimeout {
				t.Fatalf("InspectBrowser() = %#v, %v", got.result, got.err)
			}
		case <-time.After(time.Second):
			t.Fatal("InspectBrowser did not time out")
		}
		if broker.ResolveInspection(requested.ID, tools.BrowserInspectionResult{Status: tools.BrowserInspectionCompleted}) {
			t.Fatal("late inspection result was accepted")
		}
	})

	t.Run("close", func(t *testing.T) {
		hub := NewHub()
		events, _ := hub.add(0)
		defer hub.remove(events)
		broker := NewBrowserBroker(hub)
		result := make(chan browserInspectionCallResult, 1)
		go func() {
			got, err := broker.InspectBrowser(context.Background())
			result <- browserInspectionCallResult{result: got, err: err}
		}()
		requested := readBrowserEvent(t, events)
		broker.Close()

		select {
		case got := <-result:
			if got.err != nil || got.result.ID != requested.ID || got.result.Status != tools.BrowserInspectionCancelled {
				t.Fatalf("InspectBrowser() = %#v, %v", got.result, got.err)
			}
		case <-time.After(time.Second):
			t.Fatal("closed InspectBrowser did not return")
		}
	})
}

func TestBrowserInspectionResultEndpointResolvesSessionCommand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	transports := NewSessionTransports()
	transport := transports.New("session-1").(*sessionTransport)
	events, _ := transport.hub.add(0)
	defer transport.hub.remove(events)
	result := make(chan browserInspectionCallResult, 1)
	go func() {
		got, err := transport.InspectBrowser(context.Background())
		result <- browserInspectionCallResult{result: got, err: err}
	}()
	requested := readBrowserEvent(t, events)

	router := gin.New()
	server := &Server{transports: transports}
	router.POST("/api/sessions/:sessionID/browser/inspect/:commandID/result", server.handleBrowserInspectionResult)
	body := []byte(`{
		"status":"completed",
		"url":"https://example.com/final",
		"title":"Final",
		"pageStatus":"ready",
		"revision":4,
		"visibleText":"Visible page"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/sessions/session-1/browser/inspect/"+requested.ID+"/result",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}

	got := <-result
	if got.err != nil || got.result.URL != "https://example.com/final" || got.result.VisibleText != "Visible page" {
		t.Fatalf("result = %#v, %v", got.result, got.err)
	}

	duplicateRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/sessions/session-1/browser/inspect/"+requested.ID+"/result",
		bytes.NewReader(body),
	)
	duplicateRequest.Header.Set("Content-Type", "application/json")
	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, duplicateRequest)
	if duplicate.Code != http.StatusNotFound {
		t.Fatalf("duplicate response = %d", duplicate.Code)
	}
}

func TestDecodeBrowserInspectionResultValidatesAndBoundsObservation(t *testing.T) {
	tests := []struct {
		name string
		body wireBrowserInspectionResult
	}{
		{name: "status", body: wireBrowserInspectionResult{Status: "unknown"}},
		{name: "page status", body: wireBrowserInspectionResult{Status: wireBrowserInspectionFailed, PageStatus: "unknown"}},
		{name: "revision", body: wireBrowserInspectionResult{Status: wireBrowserInspectionFailed, Revision: -1}},
		{name: "ready", body: wireBrowserInspectionResult{Status: wireBrowserInspectionCompleted, PageStatus: wireBrowserPageFailed, URL: "https://example.com"}},
		{name: "URL", body: wireBrowserInspectionResult{Status: wireBrowserInspectionCompleted, PageStatus: wireBrowserPageReady, URL: "file:///tmp/page.html"}},
		{name: "credentials", body: wireBrowserInspectionResult{Status: wireBrowserInspectionCompleted, PageStatus: wireBrowserPageReady, URL: "https://user:secret@example.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeBrowserInspectionResult(test.body); err == nil {
				t.Fatalf("decodeBrowserInspectionResult(%#v) succeeded", test.body)
			}
		})
	}

	visibleText := strings.Repeat("界", tools.MaxBrowserInspectionTextRunes+10)
	body := wireBrowserInspectionResult{
		Status:      wireBrowserInspectionCompleted,
		URL:         "https://example.com/",
		Title:       strings.Repeat("T", 600),
		PageStatus:  wireBrowserPageReady,
		Revision:    2,
		VisibleText: visibleText,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 64<<10 {
		t.Fatalf("test payload unexpectedly exceeds endpoint body limit: %d", len(encoded))
	}
	got, err := decodeBrowserInspectionResult(body)
	if err != nil {
		t.Fatal(err)
	}
	if count := len([]rune(got.VisibleText)); count != tools.MaxBrowserInspectionTextRunes {
		t.Fatalf("visible text runes = %d", count)
	}
	if !got.Truncated || len([]rune(got.Title)) != 512 {
		t.Fatalf("bounded result = %#v", got)
	}
}
