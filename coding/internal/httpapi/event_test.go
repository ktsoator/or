package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/tools"
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

func TestProjectEventIncludesToolInputProgress(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:             engine.ToolInputDelta,
		ToolCallID:       "call-1",
		ToolName:         "write",
		ToolContentIndex: 0,
		ToolInputBytes:   128,
	})
	if !ok {
		t.Fatal("tool input event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "tool_input_delta" || event.ID != "call-1" || event.Tool != "write" {
		t.Fatalf("event = %#v", event)
	}
	if event.ToolContentIndex == nil || *event.ToolContentIndex != 0 {
		t.Fatalf("toolContentIndex = %#v, want 0", event.ToolContentIndex)
	}
	if event.Bytes != 128 {
		t.Fatalf("bytes = %d, want 128", event.Bytes)
	}
}

func TestProjectEventIncludesLivePreviewRequest(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "preview-call",
		ToolName:   "open_preview",
		ToolDetails: tools.PreviewRequest{
			URL:   "http://localhost:3000",
			Title: "Local app",
		},
	})
	if !ok {
		t.Fatal("preview tool event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Preview == nil || event.Preview.URL != "http://localhost:3000" || event.Preview.Title != "Local app" {
		t.Fatalf("preview = %#v", event.Preview)
	}
}

func TestProjectEventIncludesWorkspacePreviewPath(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "preview-call",
		ToolName:   "open_preview",
		ToolDetails: tools.PreviewRequest{
			Path:         "/workspace/web/index.html",
			RelativePath: "web/index.html",
			Title:        "Static page",
		},
	})
	if !ok {
		t.Fatal("preview tool event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Preview == nil || event.Preview.Path != "/workspace/web/index.html" || event.Preview.RelativePath != "web/index.html" || event.Preview.Title != "Static page" || event.Preview.URL != "" {
		t.Fatalf("preview = %#v", event.Preview)
	}
}

func TestProjectHistoryRestoresPreviewRequest(t *testing.T) {
	events := ProjectHistory([]engine.HistoryItem{{
		Type:       engine.HistoryToolResult,
		ToolCallID: "preview-call",
		ToolName:   "open_preview",
		ToolDetails: tools.PreviewRequest{
			Path:         "/workspace/web/index.html",
			RelativePath: "web/index.html",
			Title:        "Static page",
		},
	}})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one event", events)
	}
	preview := events[0].Preview
	if preview == nil || preview.Path != "/workspace/web/index.html" || preview.RelativePath != "web/index.html" || preview.Title != "Static page" {
		t.Fatalf("history preview = %#v", preview)
	}
}
