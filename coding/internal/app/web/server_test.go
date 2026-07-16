package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/store"
	"github.com/ktsoator/or/llm"
)

func TestHistoryEndpointRestoresSessionTranscript(t *testing.T) {
	ctx := context.Background()
	memory := &store.Memory{}
	if err := memory.Append(ctx,
		agent.UserMessage("hello"),
		agent.FromLLM(&llm.AssistantMessage{
			Content:    []llm.AssistantContent{&llm.TextContent{Text: "welcome"}},
			StopReason: llm.StopReasonStop,
		}),
	); err != nil {
		t.Fatal(err)
	}
	session, err := coding.New(ctx, coding.Options{
		Model: llm.Model{ID: "test", Provider: "test"},
		Cwd:   t.TempDir(),
		Store: memory,
	})
	if err != nil {
		t.Fatal(err)
	}

	hub := NewHub()
	server := NewServer(ctx, session, hub, NewConfirmBroker(hub))
	request := httptest.NewRequest(http.MethodGet, "/history", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var events []wireEvent
	if err := json.Unmarshal(response.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != "user_message" || events[1].Type != "message_end" {
		t.Fatalf("events = %+v", events)
	}
}
