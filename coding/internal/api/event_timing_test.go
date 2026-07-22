package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ktsoator/or/coding"
)

func TestProjectRunTiming(t *testing.T) {
	startedAt := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(3250 * time.Millisecond)
	data, ok := ProjectEvent(coding.Event{
		Type:        coding.RunCompleted,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	})
	if !ok {
		t.Fatal("run completion was not projected")
	}
	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "done" || event.StartedAt == "" || event.DurationMS == nil || *event.DurationMS != 3250 {
		t.Fatalf("projected event = %#v", event)
	}

	history := ProjectHistory([]coding.HistoryItem{{
		Type: coding.HistoryRun, StartedAt: startedAt, CompletedAt: startedAt,
	}})
	if len(history) != 1 || history[0].DurationMS == nil || *history[0].DurationMS != 0 {
		t.Fatalf("zero-duration history event = %#v", history)
	}
}
