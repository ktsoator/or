package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestSessionExposesBackgroundTaskLifecycle(t *testing.T) {
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	started := make(chan Event, 1)
	completed := make(chan Event, 1)
	session.Subscribe(func(event Event) {
		switch event.Type {
		case TaskStarted:
			started <- event
		case TaskCompleted:
			completed <- event
		}
	})
	info, err := session.toolRuntime.startTask(
		`printf ready; while true; do sleep 1; done`,
		"Start test server",
	)
	if err != nil {
		t.Fatal(err)
	}

	if event := <-started; event.BackgroundTask.ID != info.ID ||
		event.BackgroundTask.Status != string(tools.TaskRunning) {
		t.Fatalf("start event = %#v", event)
	}
	tasks := session.Tasks()
	if len(tasks) != 1 || tasks[0].Description != "Start test server" ||
		tasks[0].Status != string(tools.TaskRunning) {
		t.Fatalf("tasks = %#v", tasks)
	}
	if err := session.StopTask(info.ID); err != nil {
		t.Fatal(err)
	}
	if event := <-completed; event.BackgroundTask.Status != string(tools.TaskStopped) {
		t.Fatalf("completion event = %#v", event)
	}
	if _, err := session.TaskOutput("missing"); !errors.Is(err, tools.ErrTaskNotFound) {
		t.Fatalf("missing output error = %v, want ErrTaskNotFound", err)
	}
}

func TestBackgroundTaskCompletionReachesEventsAndNextModelRequest(t *testing.T) {
	var captured llm.Context
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Cwd:   t.TempDir(),
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			captured = input
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	taskEvents := make(chan Event, 1)
	session.Subscribe(func(event Event) {
		if event.Type == TaskCompleted {
			taskEvents <- event
		}
	})
	info, err := session.toolRuntime.startTask(`printf '<ready>'`, "Print readiness")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-taskEvents:
		if event.BackgroundTask.ID != info.ID || event.BackgroundTask.Status != string(tools.TaskSucceeded) ||
			event.BackgroundTask.OutputPath != info.OutputPath || event.BackgroundTask.ExitCode == nil ||
			*event.BackgroundTask.ExitCode != 0 {
			t.Fatalf("task event = %#v", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task completion event")
	}

	if err := session.Prompt(context.Background(), "what finished?"); err != nil {
		t.Fatal(err)
	}
	var taskContext string
	for _, message := range captured.Messages {
		text := llmUserText(t, message)
		if strings.Contains(text, `<or-context kind="task_status">`) {
			taskContext = text
			break
		}
	}
	for _, want := range []string{
		`<task id="task_1" status="succeeded" exit_code="0"`,
		info.OutputPath,
		"printf &#39;&lt;ready&gt;&#39;",
	} {
		if !strings.Contains(taskContext, want) {
			t.Errorf("task context missing %q:\n%s", want, taskContext)
		}
	}
}

func TestRenderTaskStatusIsBounded(t *testing.T) {
	completed := make([]tools.TaskState, maxTaskStatusEntries+2)
	for index := range completed {
		completed[index] = tools.TaskState{
			TaskInfo: tools.TaskInfo{ID: "task_" + string(rune('A'+index)), Command: "command"},
			Status:   tools.TaskSucceeded,
		}
	}
	rendered := renderTaskStatus(completed)
	if strings.Contains(rendered, `id="task_A"`) || strings.Contains(rendered, `id="task_B"`) {
		t.Fatalf("old task entries were not bounded:\n%s", rendered)
	}
	if !strings.Contains(rendered, `id="task_C"`) {
		t.Fatalf("latest task entries missing:\n%s", rendered)
	}
}
