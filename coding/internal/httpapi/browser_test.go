package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/coding/internal/tools"
)

type browserCommandResult struct {
	result tools.BrowserResult
	err    error
}

func TestBrowserBrokerResolvesFirstTerminalResult(t *testing.T) {
	hub := NewHub()
	events, syncRequired := hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	result := make(chan browserCommandResult, 1)

	go func() {
		got, err := broker.OpenBrowser(context.Background(), browserRequest())
		result <- browserCommandResult{result: got, err: err}
	}()

	requested := readBrowserEvent(t, events)
	if requested.Type != wireEventBrowserRequest || requested.ID == "" || requested.Preview == nil {
		t.Fatalf("request event = %#v", requested)
	}
	if requested.Preview.URL != "https://example.com/start" || requested.Disposition != wireBrowserReuseAgentTab {
		t.Fatalf("request event = %#v", requested)
	}
	pending := broker.PendingEvents()
	if len(pending) != 1 || pending[0].ID != requested.ID {
		t.Fatalf("pending events = %#v", pending)
	}

	terminal := tools.BrowserResult{
		Status:       tools.BrowserCommitted,
		RequestedURL: "https://example.com/start",
		CommittedURL: "https://example.com/final",
		Title:        "Final",
	}
	if !broker.Resolve(requested.ID, terminal) {
		t.Fatal("Resolve returned false")
	}
	if broker.Resolve(requested.ID, tools.BrowserResult{Status: tools.BrowserFailed}) {
		t.Fatal("duplicate result was accepted")
	}

	select {
	case got := <-result:
		if got.err != nil || got.result.Status != tools.BrowserCommitted || got.result.ID != requested.ID {
			t.Fatalf("OpenBrowser() = %#v, %v", got.result, got.err)
		}
		if got.result.CommittedURL != "https://example.com/final" {
			t.Fatalf("committed URL = %q", got.result.CommittedURL)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenBrowser did not return")
	}
	if broker.HasPending() {
		t.Fatal("broker still has a pending command")
	}
}

func TestBrowserBrokerCancelsWithRunContext(t *testing.T) {
	hub := NewHub()
	events, _ := hub.add(0)
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan browserCommandResult, 1)
	go func() {
		got, err := broker.OpenBrowser(ctx, browserRequest())
		result <- browserCommandResult{result: got, err: err}
	}()

	requested := readBrowserEvent(t, events)
	cancel()
	select {
	case got := <-result:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("OpenBrowser error = %v, want context.Canceled", got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled OpenBrowser did not return")
	}
	if broker.Resolve(requested.ID, tools.BrowserResult{Status: tools.BrowserCommitted}) {
		t.Fatal("late result was accepted")
	}
}

func TestBrowserBrokerTimesOutAndRejectsLateResult(t *testing.T) {
	hub := NewHub()
	events, _ := hub.add(0)
	defer hub.remove(events)
	broker := NewBrowserBroker(hub)
	broker.timeout = 10 * time.Millisecond
	result := make(chan browserCommandResult, 1)
	go func() {
		got, err := broker.OpenBrowser(context.Background(), browserRequest())
		result <- browserCommandResult{result: got, err: err}
	}()
	requested := readBrowserEvent(t, events)

	select {
	case got := <-result:
		if got.err != nil || got.result.Status != tools.BrowserTimeout {
			t.Fatalf("OpenBrowser() = %#v, %v", got.result, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("OpenBrowser did not time out")
	}
	if broker.Resolve(requested.ID, tools.BrowserResult{Status: tools.BrowserCommitted}) {
		t.Fatal("late result was accepted")
	}
}

func TestBrowserResultEndpointResolvesSessionCommand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	transports := NewSessionTransports()
	transport := transports.New("session-1").(*sessionTransport)
	events, _ := transport.hub.add(0)
	defer transport.hub.remove(events)
	result := make(chan browserCommandResult, 1)
	go func() {
		got, err := transport.OpenBrowser(context.Background(), browserRequest())
		result <- browserCommandResult{result: got, err: err}
	}()
	requested := readBrowserEvent(t, events)

	router := gin.New()
	server := &Server{transports: transports}
	router.POST("/api/sessions/:sessionID/browser/:commandID/result", server.handleBrowserResult)
	body := []byte(`{
		"status":"committed",
		"requestedURL":"https://example.com/start",
		"committedURL":"https://example.com/final",
		"title":"Final"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/sessions/session-1/browser/"+requested.ID+"/result",
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}

	got := <-result
	if got.err != nil || got.result.CommittedURL != "https://example.com/final" {
		t.Fatalf("result = %#v, %v", got.result, got.err)
	}

	duplicateRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/sessions/session-1/browser/"+requested.ID+"/result",
		bytes.NewReader(body),
	)
	duplicateRequest.Header.Set("Content-Type", "application/json")
	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, duplicateRequest)
	if duplicate.Code != http.StatusNotFound {
		t.Fatalf("duplicate response = %d", duplicate.Code)
	}
}

func browserRequest() tools.BrowserRequest {
	return tools.BrowserRequest{
		Preview: tools.PreviewRequest{
			URL:   "https://example.com/start",
			Title: "Example",
		},
		Disposition: tools.BrowserReuseAgentTab,
	}
}

func readBrowserEvent(t *testing.T, events <-chan hubFrame) wireEvent {
	t.Helper()
	select {
	case frame := <-events:
		var event wireEvent
		if err := json.Unmarshal(frame.data, &event); err != nil {
			t.Fatal(err)
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for browser event")
		return wireEvent{}
	}
}
