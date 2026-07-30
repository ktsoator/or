package tools

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// gracefulStopDelay is how long a background task may exit after SIGTERM
// before its process group is force-killed.
const gracefulStopDelay = 2 * time.Second

const defaultTaskOutputMaxBytes int64 = 64 * 1024

// ErrTaskNotFound is returned when a task id is not owned by this manager.
var ErrTaskNotFound = errors.New("background task not found")

// TaskStatus is the lifecycle state of a background task.
type TaskStatus string

const (
	TaskRunning   TaskStatus = "running"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
	TaskStopped   TaskStatus = "stopped"
)

// TaskInfo is returned immediately after a background task starts.
type TaskInfo struct {
	ID          string
	Command     string
	Description string
	OutputPath  string
	StartedAt   time.Time
}

// TaskState is the latest state of one managed background task. CompletedAt is
// zero and ExitCode is nil while the task is running.
type TaskState struct {
	TaskInfo
	Status      TaskStatus
	ExitCode    *int
	CompletedAt time.Time
}

// TaskOutput is a bounded tail of one task's combined stdout and stderr.
type TaskOutput struct {
	Content   string
	Truncated bool
}

type managedTask struct {
	state         TaskState
	cmd           *exec.Cmd
	done          chan struct{}
	stopRequested bool
}

// TaskManager owns background processes and their output files for one coding
// session. Tasks run in separate process groups, survive across turns, and are
// stopped and removed when the session closes.
type TaskManager struct {
	mu sync.Mutex

	counter      int
	outputDir    string
	closed       bool
	tasks        map[string]*managedTask
	listeners    map[int]func(TaskState)
	nextListener int
}

// NewTaskManager returns an empty session-scoped task manager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:     make(map[string]*managedTask),
		listeners: make(map[int]func(TaskState)),
	}
}

// Start launches command in dir, redirects its combined output to a private
// managed file, and returns without waiting for the process to exit.
func (m *TaskManager) Start(command, description, dir string) (TaskInfo, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return TaskInfo{}, errors.New("task manager is closed")
	}
	if m.outputDir == "" {
		outputDir, err := os.MkdirTemp("", "coding-tasks-*")
		if err != nil {
			m.mu.Unlock()
			return TaskInfo{}, fmt.Errorf("create task output directory: %w", err)
		}
		m.outputDir = outputDir
	}
	m.counter++
	id := "task_" + strconv.Itoa(m.counter)
	info := TaskInfo{
		ID:          id,
		Command:     command,
		Description: description,
		OutputPath:  filepath.Join(m.outputDir, id+".log"),
		StartedAt:   time.Now().UTC(),
	}
	m.mu.Unlock()

	output, err := os.OpenFile(info.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return TaskInfo{}, fmt.Errorf("create task output: %w", err)
	}

	cmd, err := newBashCommand(command, dir)
	if err != nil {
		_ = output.Close()
		_ = os.Remove(info.OutputPath)
		return TaskInfo{}, err
	}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		_ = output.Close()
		_ = os.Remove(info.OutputPath)
		return TaskInfo{}, err
	}

	task := &managedTask{
		state: TaskState{TaskInfo: info, Status: TaskRunning},
		cmd:   cmd,
		done:  make(chan struct{}),
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = terminateProcessGroup(cmd, syscall.SIGKILL)
		_ = cmd.Wait()
		_ = output.Close()
		_ = os.Remove(info.OutputPath)
		return TaskInfo{}, errors.New("task manager is closed")
	}
	m.tasks[id] = task
	listeners := m.listenersLocked()
	m.mu.Unlock()

	for _, listener := range listeners {
		listener(task.state)
	}
	go m.wait(task, output)
	return info, nil
}

func (m *TaskManager) wait(task *managedTask, output *os.File) {
	err := task.cmd.Wait()
	_ = output.Close()

	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	m.mu.Lock()
	status := TaskSucceeded
	if task.stopRequested {
		status = TaskStopped
	} else if exitCode != 0 {
		status = TaskFailed
	}
	completedAt := time.Now().UTC()
	task.state = TaskState{
		TaskInfo:    task.state.TaskInfo,
		Status:      status,
		ExitCode:    &exitCode,
		CompletedAt: completedAt,
	}
	close(task.done)
	state := task.state
	listeners := m.listenersLocked()
	m.mu.Unlock()

	for _, listener := range listeners {
		listener(state)
	}
}

// Stop terminates a running task's process group. It is a no-op for a task that
// has already finished.
func (m *TaskManager) Stop(id string) error {
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	if task.state.Status != TaskRunning {
		m.mu.Unlock()
		return nil
	}
	task.stopRequested = true
	m.mu.Unlock()

	if err := terminateProcessGroup(task.cmd, syscall.SIGTERM); err != nil {
		return err
	}
	select {
	case <-task.done:
		return nil
	case <-time.After(gracefulStopDelay):
		return terminateProcessGroup(task.cmd, syscall.SIGKILL)
	}
}

// OwnsOutputPath reports whether path is exactly one output file registered by
// this manager. It deliberately does not trust the containing directory.
func (m *TaskManager) OwnsOutputPath(path string) bool {
	if m == nil || path == "" {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absolute = filepath.Clean(absolute)

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, task := range m.tasks {
		if filepath.Clean(task.state.OutputPath) == absolute {
			return true
		}
	}
	return false
}

// Snapshot returns every task's latest state in task creation order.
func (m *TaskManager) Snapshot() []TaskState {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	states := make([]TaskState, 0, len(m.tasks))
	for _, task := range m.tasks {
		states = append(states, task.state)
	}
	sort.Slice(states, func(i, j int) bool {
		return taskNumber(states[i].ID) < taskNumber(states[j].ID)
	})
	return states
}

// Completed returns terminal task states in task creation order.
func (m *TaskManager) Completed() []TaskState {
	states := m.Snapshot()
	completed := states[:0]
	for _, state := range states {
		if state.Status != TaskRunning {
			completed = append(completed, state)
		}
	}
	return completed
}

// ReadOutput returns the latest bytes from a managed task's combined output.
func (m *TaskManager) ReadOutput(id string, maxBytes int64) (TaskOutput, error) {
	if m == nil {
		return TaskOutput{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return TaskOutput{}, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	path := task.state.OutputPath
	m.mu.Unlock()

	if maxBytes <= 0 {
		maxBytes = defaultTaskOutputMaxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return TaskOutput{}, fmt.Errorf("open task output: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return TaskOutput{}, fmt.Errorf("stat task output: %w", err)
	}
	start := info.Size() - maxBytes
	truncated := start > 0
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, 0); err != nil {
		return TaskOutput{}, fmt.Errorf("seek task output: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return TaskOutput{}, fmt.Errorf("read task output: %w", err)
	}
	return TaskOutput{Content: string(content), Truncated: truncated}, nil
}

func taskNumber(id string) int {
	prefix := "task_"
	if len(id) < len(prefix) || id[:len(prefix)] != prefix {
		return 0
	}
	value, _ := strconv.Atoi(id[len(prefix):])
	return value
}

// Subscribe registers a lifecycle listener and returns its remover.
func (m *TaskManager) Subscribe(listener func(TaskState)) func() {
	if m == nil || listener == nil {
		return func() {}
	}
	m.mu.Lock()
	id := m.nextListener
	m.nextListener++
	m.listeners[id] = listener
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.listeners, id)
		m.mu.Unlock()
	}
}

func (m *TaskManager) listenersLocked() []func(TaskState) {
	listeners := make([]func(TaskState), 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	return listeners
}

// Shutdown stops all running tasks and removes their managed output files.
func (m *TaskManager) Shutdown() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.listeners = make(map[int]func(TaskState))
	ids := make([]string, 0, len(m.tasks))
	for id, task := range m.tasks {
		if task.state.Status == TaskRunning {
			ids = append(ids, id)
		}
	}
	outputDir := m.outputDir
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.Stop(id)
	}
	if outputDir != "" {
		_ = os.RemoveAll(outputDir)
	}
}
