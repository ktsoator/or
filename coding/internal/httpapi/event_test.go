package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/tools"
)

func TestProjectSessionEventIncludesRunFailure(t *testing.T) {
	data, ok := projectSessionEvent(conversation.RunFailed{Text: "model unavailable"})
	if !ok {
		t.Fatal("run failure event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "error" || event.Text != "model unavailable" {
		t.Fatalf("event = %#v", event)
	}
}

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
			GrantID:      "preview-grant",
			PreviewPath:  "index.html",
		},
	})
	if !ok {
		t.Fatal("preview tool event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Preview == nil || event.Preview.Path != "/workspace/web/index.html" || event.Preview.RelativePath != "web/index.html" || event.Preview.Title != "Static page" || event.Preview.GrantID != "preview-grant" || event.Preview.PreviewPath != "index.html" || event.Preview.URL != "" {
		t.Fatalf("preview = %#v", event.Preview)
	}
}

func TestProjectEventIncludesStructuredFileChange(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "write-call",
		ToolName:   "write",
		ToolDetails: tools.FileChange{
			Path:      "main.go",
			Kind:      tools.ChangeUpdate,
			Additions: 2,
			Deletions: 1,
			Bytes:     42,
			Hunks: []tools.Hunk{{
				OldStart: 3,
				OldLines: 1,
				NewStart: 3,
				NewLines: 2,
				Lines:    []string{"-old", "+new", "+line"},
			}},
		},
	})
	if !ok {
		t.Fatal("file change event was not projected")
	}

	var event struct {
		Change wireFileChangePayload `json:"change"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Change.ChangeType != wireChangeFile || event.Change.Operation != wireFileUpdate {
		t.Fatalf("change = %#v", event.Change)
	}
	if event.Change.Path != "main.go" || event.Change.Additions != 2 || event.Change.Deletions != 1 || event.Change.Bytes != 42 {
		t.Fatalf("change = %#v", event.Change)
	}
	if len(event.Change.Hunks) != 1 || len(event.Change.Hunks[0].Lines) != 3 {
		t.Fatalf("hunks = %#v", event.Change.Hunks)
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
