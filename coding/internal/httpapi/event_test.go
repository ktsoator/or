package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

const multilineToolResult = `[
  {
    "name": "Github",
    "transport": "streamable_http",
    "state": "connected",
    "toolCount": 44
  },
  {
    "name": "local-test",
    "transport": "stdio",
    "state": "connected",
    "toolCount": 5
  }
]`

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

func TestProjectEventIncludesPersistedRunMessageIDs(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:               engine.RunCompleted,
		RunID:              "run-1",
		UserMessageIDs:     []string{"user-1", "user-2"},
		AssistantMessageID: "assistant-1",
	})
	if !ok {
		t.Fatal("run completion event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.RunID != "run-1" ||
		strings.Join(event.UserMessageIDs, ",") != "user-1,user-2" ||
		event.AssistantMessageID != "assistant-1" {
		t.Fatalf("run %q message ids = users %v, assistant %q", event.RunID, event.UserMessageIDs, event.AssistantMessageID)
	}
}

func TestProjectEventPreservesUnknownInputUsage(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:  engine.MessageCompleted,
		Usage: llm.Usage{InputUnknown: true, Output: 5, TotalTokens: 5},
	})
	if !ok {
		t.Fatal("message completion event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Usage == nil || !event.Usage.InputUnknown {
		t.Fatalf("usage = %#v, want inputUnknown preserved", event.Usage)
	}
}

func TestProjectEventIncludesContextBreakdown(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type: engine.MessageCompleted,
		ContextUsage: engine.ContextUsage{
			Provider: "openai", Model: "test-model", UsedTokens: 42,
			ContextWindow: 128_000, Measured: true,
			Breakdown: &engine.ContextBreakdown{
				Messages: 20, SystemTools: 8, SystemPrompt: 6, Skills: 4, ProjectContext: 4,
			},
		},
	})
	if !ok {
		t.Fatal("message completion event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Context == nil || !event.Context.Measured || event.Context.UsedTokens != 42 ||
		event.Context.Breakdown == nil || event.Context.Breakdown.Messages != 20 ||
		event.Context.Breakdown.ProjectContext != 4 {
		t.Fatalf("context = %#v", event.Context)
	}
}

func TestProjectHistoryIncludesResponseCompletionTime(t *testing.T) {
	completedAt := time.Date(2026, time.July, 22, 16, 43, 0, 0, time.UTC)
	events := ProjectHistory([]engine.HistoryItem{{
		Type:          engine.HistoryAssistant,
		MessageID:     "message-1",
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
	if events[0].MessageID != "message-1" {
		t.Fatalf("messageID = %q, want message-1", events[0].MessageID)
	}
}

func TestProjectHistoryIncludesDiagnosticRunID(t *testing.T) {
	events := ProjectHistory([]engine.HistoryItem{{
		Type:  engine.HistoryRun,
		RunID: "run-1",
	}})

	if len(events) != 1 || events[0].RunID != "run-1" || events[0].ID != "run-1" {
		t.Fatalf("events = %#v, want run-1 identity", events)
	}
}

func TestProjectHistoryIncludesUserMessageMetadata(t *testing.T) {
	sentAt := time.Date(2026, time.August, 11, 9, 30, 0, 0, time.FixedZone("PDT", -7*60*60))
	events := ProjectHistory([]engine.HistoryItem{{
		Type:      engine.HistoryUser,
		MessageID: "message-2",
		SentAt:    sentAt,
		Text:      "question",
	}})

	if len(events) != 1 || events[0].MessageID != "message-2" {
		t.Fatalf("events = %#v, want persisted user message ID", events)
	}
	if want := sentAt.UTC().Format(time.RFC3339Nano); events[0].SentAt != want {
		t.Fatalf("sentAt = %q, want %q", events[0].SentAt, want)
	}
}

func TestProjectEventIncludesUserMessageTime(t *testing.T) {
	sentAt := time.Date(2026, time.August, 11, 9, 45, 0, 0, time.UTC)
	data, ok := ProjectEvent(engine.Event{
		Type:   engine.UserMessageCompleted,
		Text:   "question",
		SentAt: sentAt,
	})
	if !ok {
		t.Fatal("user message event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.SentAt != sentAt.Format(time.RFC3339Nano) {
		t.Fatalf("sentAt = %q, want %q", event.SentAt, sentAt.Format(time.RFC3339Nano))
	}
}

func TestProjectEventIncludesToolInputProgress(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:             engine.ToolInputDelta,
		ToolCallID:       "call-1",
		ToolName:         "write",
		ToolContentIndex: 0,
		Delta:            `{"path":`,
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
	if event.Delta != `{"path":` {
		t.Fatalf("delta = %q, want streamed tool arguments", event.Delta)
	}
}

func TestProjectEventIncludesToolResultImages(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "image-call",
		ToolName:   "mcp__everything__get_tiny_image",
		Images: []llm.ImageContent{{
			Data:     "aW1hZ2U=",
			MIMEType: "image/png",
		}},
	})
	if !ok {
		t.Fatal("tool result event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if len(event.Images) != 1 || event.Images[0].Data != "aW1hZ2U=" || event.Images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", event.Images)
	}
}

func TestProjectEventPreservesCompleteToolResult(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "status-call",
		ToolName:   "mcp_status",
		ToolResult: multilineToolResult,
	})
	if !ok {
		t.Fatal("tool result event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Result != multilineToolResult {
		t.Fatalf("result = %q, want complete tool result", event.Result)
	}
}

func TestWireToolResultReportsTransportLimits(t *testing.T) {
	t.Run("lines", func(t *testing.T) {
		result := strings.Repeat("line\n", wireToolResultMaxLines) + "last line"
		got := wireToolResult(result)
		if strings.Contains(got, "last line") ||
			!strings.HasSuffix(got, "[truncated: 1 more line(s) not shown]") {
			t.Fatalf("result did not report the line limit: %q", got)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		got := wireToolResult(strings.Repeat("界", wireToolResultMaxBytes))
		if len(got) > wireToolResultMaxBytes {
			t.Fatalf("result length = %d, want <= %d", len(got), wireToolResultMaxBytes)
		}
		if !utf8.ValidString(got) {
			t.Fatal("byte-limited result is not valid UTF-8")
		}
		if !strings.HasSuffix(got, "[truncated: tool result exceeded 65536 bytes]") {
			t.Fatalf("result did not report the byte limit: %q", got)
		}
	})
}

func TestProjectEventIncludesTaskCompletion(t *testing.T) {
	completedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	exitCode := 1
	data, ok := ProjectEvent(engine.Event{
		Type: engine.TaskCompleted,
		BackgroundTask: engine.BackgroundTask{
			ID:          "task_2",
			Status:      "failed",
			OutputPath:  "/tmp/coding-tasks/task_2.log",
			Command:     "go test ./...",
			ExitCode:    &exitCode,
			CompletedAt: completedAt,
		},
	})
	if !ok {
		t.Fatal("task completion event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != wireEventTaskNotification || event.Task == nil ||
		event.Task.ID != "task_2" || event.Task.Status != wireTaskFailed ||
		event.Task.ExitCode == nil || *event.Task.ExitCode != 1 ||
		event.Task.OutputPath != "/tmp/coding-tasks/task_2.log" ||
		event.Task.CompletedAt != completedAt.Format(time.RFC3339Nano) {
		t.Fatalf("event = %#v", event)
	}
}

func TestProjectEventIncludesTaskStart(t *testing.T) {
	startedAt := time.Date(2026, time.July, 25, 11, 59, 0, 0, time.UTC)
	data, ok := ProjectEvent(engine.Event{
		Type: engine.TaskStarted,
		BackgroundTask: engine.BackgroundTask{
			ID:          "task_1",
			Command:     "bun run dev",
			Description: "Start development server",
			Status:      "running",
			OutputPath:  "/tmp/coding-tasks/task_1.log",
			StartedAt:   startedAt,
		},
	})
	if !ok {
		t.Fatal("task start event was not projected")
	}

	var event wireEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != wireEventTaskStarted || event.Task == nil ||
		event.Task.ID != "task_1" || event.Task.Status != wireTaskRunning ||
		event.Task.Description != "Start development server" ||
		event.Task.StartedAt != startedAt.Format(time.RFC3339Nano) ||
		event.Task.ExitCode != nil || event.Task.CompletedAt != "" {
		t.Fatalf("event = %#v", event)
	}
}

func TestProjectEventIncludesLivePreviewRequest(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "preview-call",
		ToolName:   "open_preview",
		ToolOutcome: agent.ToolOutcome{
			Status: agent.ToolOutcomeSuccess,
			Data: tools.PreviewRequest{
				URL:   "http://localhost:3000",
				Title: "Local app",
			},
		},
	})
	if !ok {
		t.Fatal("preview tool event was not projected")
	}

	var event struct {
		Outcome struct {
			Status wireToolOutcomeStatus `json:"status"`
			Data   wirePreview           `json:"data"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Outcome.Status != wireToolOutcomeSuccess {
		t.Fatalf("outcome = %#v", event.Outcome)
	}
	if event.Outcome.Data.URL != "http://localhost:3000" || event.Outcome.Data.Title != "Local app" {
		t.Fatalf("outcome data = %#v", event.Outcome.Data)
	}
}

func TestProjectEventIncludesWorkspacePreviewPath(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "preview-call",
		ToolName:   "open_preview",
		ToolOutcome: agent.ToolOutcome{
			Status: agent.ToolOutcomeSuccess,
			Data: tools.PreviewRequest{
				Path:         "/workspace/web/index.html",
				RelativePath: "web/index.html",
				Title:        "Static page",
				GrantID:      "preview-grant",
				PreviewPath:  "index.html",
			},
		},
	})
	if !ok {
		t.Fatal("preview tool event was not projected")
	}

	var event struct {
		Outcome struct {
			Data wirePreview `json:"data"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	preview := event.Outcome.Data
	if preview.Path != "/workspace/web/index.html" || preview.RelativePath != "web/index.html" || preview.Title != "Static page" || preview.GrantID != "preview-grant" || preview.PreviewPath != "index.html" || preview.URL != "" {
		t.Fatalf("outcome data = %#v", event.Outcome.Data)
	}
}

func TestProjectEventIncludesStructuredFileChange(t *testing.T) {
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "write-call",
		ToolName:   "write",
		ToolOutcome: agent.ToolOutcome{
			Status: agent.ToolOutcomeSuccess,
			Data: tools.FileChange{
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
		},
	})
	if !ok {
		t.Fatal("file change event was not projected")
	}

	var event struct {
		Outcome struct {
			Data wireFileChangePayload `json:"data"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	change := event.Outcome.Data
	if change.ChangeType != wireChangeFile || change.Operation != wireFileUpdate {
		t.Fatalf("change = %#v", change)
	}
	if change.Path != "main.go" || change.Additions != 2 || change.Deletions != 1 || change.Bytes != 42 {
		t.Fatalf("change = %#v", change)
	}
	if len(change.Hunks) != 1 || len(change.Hunks[0].Lines) != 3 {
		t.Fatalf("hunks = %#v", change.Hunks)
	}
}

func TestProjectEventIncludesFailedToolOutcomeMetadata(t *testing.T) {
	exitCode := 2
	data, ok := ProjectEvent(engine.Event{
		Type:       engine.ToolFinished,
		ToolCallID: "bash-call",
		ToolName:   "bash",
		ToolResult: "exit status 2",
		ToolOutcome: agent.ToolOutcome{
			Status:    agent.ToolOutcomeFailed,
			ErrorCode: "command_exit_nonzero",
			ExitCode:  &exitCode,
			Data:      map[string]any{"stderr": "compile failed"},
		},
	})
	if !ok {
		t.Fatal("failed tool event was not projected")
	}

	var event struct {
		Outcome struct {
			Status    wireToolOutcomeStatus `json:"status"`
			ErrorCode string                `json:"errorCode"`
			ExitCode  *int                  `json:"exitCode"`
			Data      map[string]any        `json:"data"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Outcome.Status != wireToolOutcomeFailed ||
		event.Outcome.ErrorCode != "command_exit_nonzero" ||
		event.Outcome.ExitCode == nil || *event.Outcome.ExitCode != 2 ||
		event.Outcome.Data["stderr"] != "compile failed" {
		t.Fatalf("outcome = %#v", event.Outcome)
	}
}

func TestProjectHistoryRestoresPreviewRequest(t *testing.T) {
	events := ProjectHistory([]engine.HistoryItem{{
		Type:       engine.HistoryToolResult,
		ToolCallID: "preview-call",
		ToolName:   "open_preview",
		ToolOutcome: agent.ToolOutcome{
			Status: agent.ToolOutcomeSuccess,
			Data: tools.PreviewRequest{
				Path:         "/workspace/web/index.html",
				RelativePath: "web/index.html",
				Title:        "Static page",
			},
		},
	}})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one event", events)
	}
	preview, ok := events[0].Outcome.Data.(*wirePreview)
	if !ok || preview.Path != "/workspace/web/index.html" || preview.RelativePath != "web/index.html" || preview.Title != "Static page" {
		t.Fatalf("history outcome data = %#v", events[0].Outcome.Data)
	}
}

func TestProjectHistoryRestoresToolResultImages(t *testing.T) {
	events := ProjectHistory([]engine.HistoryItem{{
		Type:       engine.HistoryToolResult,
		ToolCallID: "image-call",
		ToolName:   "mcp__everything__get_tiny_image",
		Images: []llm.ImageContent{{
			Data:     "aW1hZ2U=",
			MIMEType: "image/png",
		}},
	}})
	if len(events) != 1 || len(events[0].Images) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Images[0].Data != "aW1hZ2U=" || events[0].Images[0].MIMEType != "image/png" {
		t.Fatalf("images = %#v", events[0].Images)
	}
}

func TestProjectHistoryPreservesCompleteToolResult(t *testing.T) {
	events := ProjectHistory([]engine.HistoryItem{{
		Type:       engine.HistoryToolResult,
		ToolCallID: "status-call",
		ToolName:   "mcp_status",
		ToolResult: multilineToolResult,
	}})
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one event", events)
	}
	if events[0].Result != multilineToolResult {
		t.Fatalf("result = %q, want complete tool result", events[0].Result)
	}
}
