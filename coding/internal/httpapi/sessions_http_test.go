package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

func TestBindCreateSessionRequestWithUnknownContentLength(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{
		"scope":"chat",
		"provider":"deepseek",
		"model":"deepseek-v4-flash",
		"thinkingLevel":"high",
		"permissionMode":"ask"
	}`))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = request

	body, ok := bindCreateSessionRequest(ctx)
	if !ok {
		t.Fatalf("bindCreateSessionRequest rejected body with unknown content length: status %d", recorder.Code)
	}
	if body.Scope != "chat" || body.Provider != "deepseek" || body.Model != "deepseek-v4-flash" {
		t.Fatalf("body = %#v", body)
	}
	if body.ThinkingLevel != "high" || body.PermissionMode != "ask" {
		t.Fatalf("settings = thinking %q, permission %q", body.ThinkingLevel, body.PermissionMode)
	}
}

func TestHistoryHTTPReturnsCurrentTodoSnapshotAndClearsItOnNextTurn(t *testing.T) {
	var modelCalls atomic.Int64
	manager, transports, model, thinking := newForkHTTPManager(t, func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		message := llm.NewAssistantMessage(model)
		if modelCalls.Add(1) == 1 {
			message.StopReason = llm.StopReasonToolUse
			message.Content = []llm.AssistantContent{&llm.ToolCall{
				ID: "todo-call", Name: tools.ToolNameTodoWrite,
				Arguments: map[string]any{"todos": []any{
					map[string]any{"content": "Inspect parser", "status": "completed"},
					map[string]any{"content": "Run tests", "status": "in_progress"},
				}},
			}}
		} else {
			message.StopReason = llm.StopReasonStop
			message.Content = []llm.AssistantContent{&llm.TextContent{Text: "done"}}
		}
		events := make(chan llm.Event, 1)
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	})
	created, err := manager.Create(
		"Todo history", t.TempDir(), conversation.ScopeProject, model, thinking, permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(Options{Conversations: manager, Transports: transports}).Handler()

	if err := manager.StartPromptWithFiles(created.ID, "finish the parser", nil); err != nil {
		t.Fatal(err)
	}
	waitForHTTPTestSessionIdle(t, manager, created.ID)
	response := sessionHistoryRequest(handler, created.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history wireHistoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Todos == nil || len(history.Todos.Todos) != 2 ||
		history.Todos.Todos[0].Content != "Inspect parser" ||
		history.Todos.Todos[1].Status != "in_progress" {
		t.Fatalf("history todos = %#v", history.Todos)
	}
	assertHistoryTodoJSON(t, response, false)

	if err := manager.StartPromptWithFiles(created.ID, "say what is next", nil); err != nil {
		t.Fatal(err)
	}
	waitForHTTPTestSessionIdle(t, manager, created.ID)
	response = sessionHistoryRequest(handler, created.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("next history status = %d, body = %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Todos != nil {
		t.Fatalf("next-turn todos = %#v, want nil", history.Todos)
	}
	assertHistoryTodoJSON(t, response, true)
}

func TestPlanModeHTTPUpdatesHistory(t *testing.T) {
	manager, transports, model, thinking := newForkHTTPManager(t, func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		message := llm.NewAssistantMessage(model)
		message.StopReason = llm.StopReasonStop
		message.Content = []llm.AssistantContent{&llm.TextContent{Text: "done"}}
		events := make(chan llm.Event, 1)
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	})
	created, err := manager.Create(
		"Plan mode", t.TempDir(), conversation.ScopeProject, model, thinking, permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(Options{Conversations: manager, Transports: transports}).Handler()

	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/sessions/"+created.ID+"/plan-mode",
		strings.NewReader(`{"active":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("plan mode status = %d, body = %s", response.Code, response.Body.String())
	}

	historyResponse := sessionHistoryRequest(handler, created.ID)
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", historyResponse.Code, historyResponse.Body.String())
	}
	var history wireHistoryResponse
	if err := json.Unmarshal(historyResponse.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if !history.PlanMode {
		t.Fatalf("history plan mode = %v, want true", history.PlanMode)
	}
}

func TestForkSessionHTTP(t *testing.T) {
	manager, transports, model, thinking := newForkHTTPManager(t, func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		message := llm.NewAssistantMessage(model)
		message.Content = []llm.AssistantContent{&llm.TextContent{Text: "answer"}}
		message.StopReason = llm.StopReasonStop
		events := make(chan llm.Event, 1)
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	})
	source, err := manager.Create("Source", t.TempDir(), conversation.ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForHTTPTestSessionIdle(t, manager, source.ID)
	snapshot, err := manager.Snapshot(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	var userID, assistantID string
	for _, item := range snapshot.History {
		switch item.Type {
		case engine.HistoryUser:
			userID = item.MessageID
		case engine.HistoryAssistant:
			if item.FinalResponse {
				assistantID = item.MessageID
			}
		}
	}
	if userID == "" || assistantID == "" {
		t.Fatalf("history has no stable message IDs: %+v", snapshot.History)
	}

	handler := NewServer(Options{Conversations: manager, Transports: transports}).Handler()
	response := forkHTTPRequest(t, handler, source.ID, map[string]any{
		"messageID": assistantID,
		"mode":      "after_assistant",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("fork status = %d, body = %s", response.Code, response.Body.String())
	}
	var child conversation.Summary
	if err := json.Unmarshal(response.Body.Bytes(), &child); err != nil {
		t.Fatal(err)
	}
	if child.ID == "" || child.ID == source.ID || child.ForkedFromSessionID != source.ID ||
		child.ForkedFromMessageID != assistantID {
		t.Fatalf("fork response = %+v", child)
	}

	tests := []struct {
		name      string
		sessionID string
		body      any
		want      int
	}{
		{
			name:      "session not found",
			sessionID: "missing",
			body:      map[string]any{"messageID": assistantID, "mode": "after_assistant"},
			want:      http.StatusNotFound,
		},
		{
			name:      "message not found",
			sessionID: source.ID,
			body:      map[string]any{"messageID": "missing", "mode": "after_assistant"},
			want:      http.StatusNotFound,
		},
		{
			name:      "invalid boundary",
			sessionID: source.ID,
			body:      map[string]any{"messageID": userID, "mode": "after_assistant"},
			want:      http.StatusBadRequest,
		},
		{
			name:      "invalid mode",
			sessionID: source.ID,
			body:      map[string]any{"messageID": userID, "mode": "sideways"},
			want:      http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := forkHTTPRequest(t, handler, test.sessionID, test.body)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d, body = %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestForkSessionHTTPRejectsActiveSource(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	manager, transports, model, thinking := newForkHTTPManager(t, func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		close(started)
		<-release
		message := llm.NewAssistantMessage(model)
		message.Content = []llm.AssistantContent{&llm.TextContent{Text: "answer"}}
		message.StopReason = llm.StopReasonStop
		events := make(chan llm.Event, 1)
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	})
	source, err := manager.Create("Active", t.TempDir(), conversation.ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("model stream did not start")
	}

	handler := NewServer(Options{Conversations: manager, Transports: transports}).Handler()
	response := forkHTTPRequest(t, handler, source.ID, map[string]any{
		"messageID": "message",
		"mode":      "after_assistant",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusConflict, response.Body.String())
	}
	close(release)
	waitForHTTPTestSessionIdle(t, manager, source.ID)
}

func TestEditMessageHTTPKeepsSessionIdentity(t *testing.T) {
	responses := []string{"old answer", "new answer"}
	manager, transports, model, thinking := newForkHTTPManager(t, func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		text := responses[0]
		responses = responses[1:]
		message := llm.NewAssistantMessage(model)
		message.Content = []llm.AssistantContent{&llm.TextContent{Text: text}}
		message.StopReason = llm.StopReasonStop
		events := make(chan llm.Event, 1)
		events <- llm.Event{Type: llm.EventDone, Message: &message}
		close(events)
		return events, nil
	})
	source, err := manager.Create("Source", t.TempDir(), conversation.ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForHTTPTestSessionIdle(t, manager, source.ID)
	snapshot, err := manager.Snapshot(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	var userID string
	for _, item := range snapshot.History {
		if item.Type == engine.HistoryUser {
			userID = item.MessageID
			break
		}
	}
	if userID == "" {
		t.Fatal("history has no editable user message")
	}

	handler := NewServer(Options{Conversations: manager, Transports: transports}).Handler()
	response := sessionPostRequest(t, handler, source.ID, "/message-edits", map[string]any{
		"messageID": userID,
		"text":      "edited question",
	})
	if response.Code != http.StatusAccepted {
		t.Fatalf("edit status = %d, body = %s", response.Code, response.Body.String())
	}
	var updated conversation.Summary
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != source.ID || !updated.Running || len(manager.List()) != 1 {
		t.Fatalf("edited session = %+v, sessions = %+v", updated, manager.List())
	}
	waitForHTTPTestSessionIdle(t, manager, source.ID)
	snapshot, err = manager.Snapshot(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, item := range snapshot.History {
		if item.Type == engine.HistoryUser || item.Type == engine.HistoryAssistant {
			texts = append(texts, item.Text)
		}
	}
	if strings.Join(texts, "|") != "edited question|new answer" {
		t.Fatalf("edited history = %q", texts)
	}
}

func newForkHTTPManager(
	t *testing.T,
	streamFn agent.StreamFn,
) (*conversation.Manager, *SessionTransports, llm.Model, llm.ModelThinkingLevel) {
	t.Helper()
	dataDir := t.TempDir()
	home := filepath.Join(dataDir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	ledger, err := usage.NewStore(filepath.Join(dataDir, "usage", "events.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := workspace.NewRegistry(filepath.Join(dataDir, "sessions", "workspaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	transports := NewSessionTransports()
	manager, err := conversation.NewManager(context.Background(), conversation.Options{
		DataDir:      dataDir,
		Usage:        ledger,
		Workspaces:   workspaces,
		NewTransport: transports.New,
		StreamFn:     streamFn,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		manager.Close()
		_ = ledger.Close()
	})
	for _, provider := range llm.GetProviders() {
		models := llm.GetModels(provider)
		if len(models) == 0 {
			continue
		}
		levels := llm.SupportedThinkingLevels(models[0])
		if len(levels) > 0 {
			return manager, transports, models[0], levels[0]
		}
	}
	t.Fatal("built-in model catalog is empty")
	return nil, nil, llm.Model{}, ""
}

func forkHTTPRequest(t *testing.T, handler http.Handler, sessionID string, body any) *httptest.ResponseRecorder {
	return sessionPostRequest(t, handler, sessionID, "/forks", body)
}

func sessionPostRequest(
	t *testing.T,
	handler http.Handler,
	sessionID, suffix string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/sessions/"+sessionID+suffix,
		bytes.NewReader(encoded),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sessionHistoryRequest(handler http.Handler, sessionID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID+"/history", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertHistoryTodoJSON(t *testing.T, response *httptest.ResponseRecorder, wantNull bool) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	raw, ok := fields["todos"]
	if !ok {
		t.Fatal("history response omitted todos")
	}
	if gotNull := string(raw) == "null"; gotNull != wantNull {
		t.Fatalf("history todos JSON = %s, want null = %v", raw, wantNull)
	}
}

func waitForHTTPTestSessionIdle(t *testing.T, manager *conversation.Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, session := range manager.List() {
			if session.ID == id && !session.Running {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("session %s did not become idle", id)
}
