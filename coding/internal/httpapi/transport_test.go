package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
)

func TestSessionTransportsRemovesClosedTransport(t *testing.T) {
	transports := NewSessionTransports()
	created := transports.New("session-1")
	transport, ok := transports.get("session-1")
	if !ok {
		t.Fatal("created transport was not registered")
	}
	client, syncRequired := transport.hub.add(0)
	if syncRequired {
		t.Fatal("unexpected sync requirement")
	}

	created.Close()
	created.Close()

	if _, ok := transports.get("session-1"); ok {
		t.Fatal("closed transport remains registered")
	}
	if _, ok := <-client; ok {
		t.Fatal("closing transport did not disconnect its client")
	}
}

func TestSessionTransportsReplacementSurvivesOldClose(t *testing.T) {
	transports := NewSessionTransports()
	first := transports.New("session-1")
	second := transports.New("session-1")

	first.Close()

	got, ok := transports.get("session-1")
	if !ok || got != second {
		t.Fatal("closing replaced transport removed the current transport")
	}
	second.Close()
}

func TestProjectTitleGenerationEvent(t *testing.T) {
	data, ok := projectSessionEvent(conversation.TitleGenerationChanged{
		Generation: conversation.TitleGeneration{
			Status:      conversation.TitleGenerationFailed,
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			ErrorCode:   "title_request_failed",
			Error:       "The utility model could not generate a title.",
			AttemptedAt: "2026-07-26T12:00:00Z",
		},
	})
	if !ok {
		t.Fatal("title generation event was not projected")
	}
	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != wireEventTitleGeneration ||
		event.TitleGenerationStatus != wireTitleGenerationFailed ||
		event.TitleGenerationErrorCode != "title_request_failed" ||
		event.TitleGenerationProvider != "openai" ||
		event.TitleGenerationModel != "gpt-4o-mini" {
		t.Fatalf("projected event = %#v", event)
	}
}

func TestSessionTransportPublishesAndSnapshotsOneActiveRun(t *testing.T) {
	transport := &sessionTransport{hub: NewHub()}
	transport.PublishAgent(engine.Event{
		Type:      engine.RunStarted,
		StartedAt: mustEventTime(t, "2026-07-27T12:00:00Z"),
	})
	transport.PublishAgent(engine.Event{Type: engine.TextDelta, Delta: "partial answer"})

	var active activeRunSnapshot
	sequence := transport.hub.snapshot(func() { active = transport.activeRun.snapshot() })
	if sequence != 2 || len(active.events) != 2 {
		t.Fatalf("sequence = %d, active events = %#v", sequence, active.events)
	}
	if active.events[1].Type != wireEventDelta || active.events[1].Delta != "partial answer" {
		t.Fatalf("streaming event = %#v", active.events[1])
	}

	transport.PublishAgent(engine.Event{Type: engine.RunCompleted})
	transport.hub.snapshot(func() { active = transport.activeRun.snapshot() })
	if active.startedAt != "" || len(active.events) != 0 {
		t.Fatalf("active run survived completion: %#v", active)
	}
}

func mustEventTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
