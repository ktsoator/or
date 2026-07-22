package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
)

func TestProjectEventIncludesResponseCompletionTime(t *testing.T) {
	completedAt := time.Date(2026, time.July, 22, 9, 42, 3, 123000000, time.FixedZone("PDT", -7*60*60))
	data, ok := ProjectEvent(engine.Event{
		Type:        engine.MessageCompleted,
		Text:        "answer",
		CompletedAt: completedAt,
	})
	if !ok {
		t.Fatal("message completion event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if want := completedAt.UTC().Format(time.RFC3339Nano); event.CompletedAt != want {
		t.Fatalf("completedAt = %q, want %q", event.CompletedAt, want)
	}
}

func TestProjectHistoryIncludesResponseCompletionTime(t *testing.T) {
	completedAt := time.Date(2026, time.July, 22, 16, 43, 0, 0, time.UTC)
	events := ProjectHistory([]engine.HistoryItem{{
		Type:          engine.HistoryAssistant,
		Text:          "answer",
		FinalResponse: true,
		CompletedAt:   completedAt,
	}})

	if len(events) != 1 {
		t.Fatalf("events = %#v, want one event", events)
	}
	if want := completedAt.Format(time.RFC3339Nano); events[0].CompletedAt != want {
		t.Fatalf("completedAt = %q, want %q", events[0].CompletedAt, want)
	}
}
