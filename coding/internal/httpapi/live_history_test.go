package httpapi

import "testing"

func TestActiveRunHistoryCompactsStreamingProgressAndClearsOnDone(t *testing.T) {
	history := activeRunHistory{}
	contentIndex := 2
	history.apply(wireEvent{Type: wireEventRunStart, StartedAt: "2026-07-27T12:00:00Z"})
	history.apply(wireEvent{Type: wireEventDelta, Kind: wireDeltaText, Delta: "still "})
	history.apply(wireEvent{Type: wireEventDelta, Kind: wireDeltaText, Delta: "working"})
	history.apply(wireEvent{
		Type: wireEventToolInputStart, Tool: "write", ToolContentIndex: &contentIndex,
	})
	history.apply(wireEvent{
		Type: wireEventToolInputDelta, Tool: "write", ToolContentIndex: &contentIndex, Bytes: 8,
	})
	history.apply(wireEvent{
		Type: wireEventToolInputDelta, Tool: "write", ToolContentIndex: &contentIndex, Bytes: 13,
	})

	snapshot := history.snapshot()
	if snapshot.startedAt != "2026-07-27T12:00:00Z" {
		t.Fatalf("startedAt = %q", snapshot.startedAt)
	}
	if len(snapshot.events) != 4 {
		t.Fatalf("events = %#v, want run, text, tool start, and tool progress", snapshot.events)
	}
	if snapshot.events[1].Delta != "still working" {
		t.Fatalf("compacted text = %q", snapshot.events[1].Delta)
	}
	if snapshot.events[3].Bytes != 21 {
		t.Fatalf("compacted tool bytes = %d", snapshot.events[3].Bytes)
	}

	history.apply(wireEvent{Type: wireEventDone})
	if cleared := history.snapshot(); cleared.startedAt != "" || len(cleared.events) != 0 {
		t.Fatalf("history after done = %#v", cleared)
	}
}

func TestMergeActiveRunHistoryReplacesMutableRunProjection(t *testing.T) {
	startedAt := "2026-07-27T12:00:00Z"
	stable := []wireEvent{
		{Type: wireEventUserMessage, Text: "earlier"},
		{Type: wireEventRunStart, StartedAt: "2026-07-27T11:00:00Z"},
		{Type: wireEventMessageEnd, Text: "earlier answer", Final: true},
		{Type: wireEventUserMessage, Text: "current prompt"},
		{Type: wireEventRunStart, StartedAt: startedAt},
		// The engine can already contain this completed message while its SSE
		// publication is blocked behind a concurrent history snapshot.
		{Type: wireEventMessageEnd, Text: "ahead of hub", Final: true},
	}
	active := activeRunSnapshot{
		startedAt: startedAt,
		events: []wireEvent{
			{Type: wireEventRunStart, StartedAt: startedAt},
			{Type: wireEventDelta, Kind: wireDeltaText, Delta: "published partial"},
		},
	}

	merged := mergeActiveRunHistory(stable, active)
	if len(merged) != 6 {
		t.Fatalf("merged events = %#v", merged)
	}
	if merged[3].Type != wireEventUserMessage || merged[3].Text != "current prompt" {
		t.Fatalf("current user message was not retained: %#v", merged)
	}
	if merged[4].Type != wireEventRunStart || merged[5].Delta != "published partial" {
		t.Fatalf("active run was not replaced with the published snapshot: %#v", merged)
	}
}

func TestHubSnapshotsActiveRunAtThePublishedSequence(t *testing.T) {
	hub := NewHub()
	history := activeRunHistory{}
	event := wireEvent{Type: wireEventRunStart, StartedAt: "2026-07-27T12:00:00Z"}
	hub.broadcast([]byte(`{"type":"run_start"}`), func() { history.apply(event) })

	var snapshot activeRunSnapshot
	sequence := hub.snapshot(func() { snapshot = history.snapshot() })
	if sequence != 1 || len(snapshot.events) != 1 || snapshot.events[0].StartedAt != event.StartedAt {
		t.Fatalf("sequence = %d, snapshot = %#v", sequence, snapshot)
	}
}
