package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/llm"
)

func TestTaskManagerWritesOutputAndNotifiesOnce(t *testing.T) {
	manager := newTaskManager()
	defer manager.Shutdown()

	notifications := make(chan TaskState, 3)
	manager.Subscribe(func(state TaskState) { notifications <- state })
	info, err := manager.Start(`printf stdout; printf stderr >&2; exit 7`, "Fail test task", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	running := awaitTaskState(t, notifications)
	if running.Status != TaskRunning || running.ID != info.ID {
		t.Fatalf("running state = %#v", running)
	}
	notification := awaitTaskState(t, notifications)
	if notification.ID != info.ID || notification.OutputPath != info.OutputPath {
		t.Fatalf("notification = %#v, info = %#v", notification, info)
	}
	if notification.Status != TaskFailed || notification.ExitCode == nil || *notification.ExitCode != 7 {
		t.Fatalf("terminal state = %#v, want failed/7", notification)
	}
	content, err := os.ReadFile(info.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "stdoutstderr" {
		t.Fatalf("output = %q", got)
	}
	select {
	case duplicate := <-notifications:
		t.Fatalf("duplicate notification = %#v", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTaskManagerStopTerminatesTask(t *testing.T) {
	manager := newTaskManager()
	defer manager.Shutdown()

	notifications := make(chan TaskState, 2)
	manager.Subscribe(func(state TaskState) { notifications <- state })
	info, err := manager.Start(`while true; do sleep 1; done`, "Run forever", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if state := awaitTaskState(t, notifications); state.Status != TaskRunning {
		t.Fatalf("initial status = %q, want running", state.Status)
	}
	snapshot := manager.Snapshot()
	if len(snapshot) != 1 || snapshot[0].ID != info.ID || snapshot[0].Status != TaskRunning ||
		snapshot[0].Description != "Run forever" || snapshot[0].StartedAt.IsZero() {
		t.Fatalf("running snapshot = %#v", snapshot)
	}
	if err := manager.Stop(info.ID); err != nil {
		t.Fatal(err)
	}
	if notification := awaitTaskState(t, notifications); notification.Status != TaskStopped {
		t.Fatalf("status = %q, want stopped", notification.Status)
	}
}

func TestTaskManagerReadsBoundedOutputTail(t *testing.T) {
	manager := newTaskManager()
	defer manager.Shutdown()

	states := make(chan TaskState, 2)
	manager.Subscribe(func(state TaskState) { states <- state })
	info, err := manager.Start(`printf 0123456789; sleep 2`, "Write test output", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = awaitTaskState(t, states)

	deadline := time.Now().Add(2 * time.Second)
	var output TaskOutput
	for time.Now().Before(deadline) {
		output, err = manager.ReadOutput(info.ID, 4)
		if err == nil && output.Content == "6789" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if output.Content != "6789" || !output.Truncated {
		t.Fatalf("output = %#v, want truncated tail", output)
	}
	if _, err := manager.ReadOutput("missing", 4); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing output error = %v, want ErrTaskNotFound", err)
	}
}

func TestTaskManagerTrustsOnlyRegisteredOutputFilesAndCleansUp(t *testing.T) {
	manager := newTaskManager()
	info, err := manager.Start("true", "Run true", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !manager.OwnsOutputPath(info.OutputPath) {
		t.Fatal("registered output path was not recognized")
	}
	if manager.OwnsOutputPath(filepath.Join(filepath.Dir(info.OutputPath), "other.log")) {
		t.Fatal("unregistered file in output directory was trusted")
	}

	manager.Shutdown()
	if _, err := os.Stat(info.OutputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output remains after shutdown: %v", err)
	}
}

func TestBackgroundBashReturnsTaskAndOutputPath(t *testing.T) {
	manager := newTaskManager()
	defer manager.Shutdown()
	tool := bashTool(t.TempDir(), manager)
	raw := json.RawMessage(`{"command":"printf ready","description":"Start test task","run_in_background":true}`)
	result, err := tool.Execute(context.Background(), "call-1", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := toolResultText(t, result.Content)
	if !strings.Contains(text, "Started background task task_1") ||
		!strings.Contains(text, "task_1.log") ||
		!strings.Contains(text, "Completion will be reported automatically") {
		t.Fatalf("result = %q", text)
	}
}

func awaitTaskState(t *testing.T, notifications <-chan TaskState) TaskState {
	t.Helper()
	select {
	case state := <-notifications:
		return state
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task completion")
		return TaskState{}
	}
}

func toolResultText(t *testing.T, content []llm.ToolResultContent) string {
	t.Helper()
	if len(content) != 1 {
		t.Fatalf("result content = %#v", content)
	}
	text, ok := content[0].(*llm.TextContent)
	if !ok {
		t.Fatalf("result content type = %T", content[0])
	}
	return text.Text
}
