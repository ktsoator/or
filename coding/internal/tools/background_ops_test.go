package tools

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// recordingExecOps stands in for a backend that sandboxes, containerizes, or
// forwards commands. Background tasks must reach it the same way foreground
// ones do, or asking for a background task would be a way around it.
type recordingExecOps struct {
	mu       sync.Mutex
	commands []string
	dirs     []string
	procs    []*fakeProcess
}

func (r *recordingExecOps) Exec(context.Context, string, string) (ExecResult, error) {
	return ExecResult{}, nil
}

func (r *recordingExecOps) Start(command string, dir string, out io.Writer) (Process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, command)
	r.dirs = append(r.dirs, dir)
	proc := &fakeProcess{exited: make(chan struct{})}
	r.procs = append(r.procs, proc)
	// Prove the manager handed over a writer that reaches the task's output
	// file rather than discarding it.
	_, _ = io.WriteString(out, "output via the seam\n")
	return proc, nil
}

func (r *recordingExecOps) snapshot() ([]string, []string, []*fakeProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.commands...), append([]string(nil), r.dirs...), append([]*fakeProcess(nil), r.procs...)
}

type fakeProcess struct {
	once     sync.Once
	exited   chan struct{}
	mu       sync.Mutex
	stopped  bool
	killed   bool
	exitCode int
}

func (p *fakeProcess) Wait() (int, error) {
	<-p.exited
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode, nil
}

func (p *fakeProcess) Stop() error {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.finish(0)
	return nil
}

func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	p.killed = true
	p.mu.Unlock()
	p.finish(-1)
	return nil
}

func (p *fakeProcess) finish(code int) {
	p.once.Do(func() {
		p.mu.Lock()
		p.exitCode = code
		p.mu.Unlock()
		close(p.exited)
	})
}

func (p *fakeProcess) wasStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

func TestBackgroundTasksRunThroughTheExecSeam(t *testing.T) {
	ops := &recordingExecOps{}
	manager := NewTaskManager(ops)
	defer manager.Shutdown()

	dir := t.TempDir()
	info, err := manager.Start("sleep 30", "Wait a while", dir)
	if err != nil {
		t.Fatal(err)
	}

	commands, dirs, procs := ops.snapshot()
	if len(commands) != 1 || commands[0] != "sleep 30" {
		t.Fatalf("commands reaching the seam = %v, want exactly the background command", commands)
	}
	if len(dirs) != 1 || dirs[0] != dir {
		t.Fatalf("dirs reaching the seam = %v, want %q", dirs, dir)
	}

	output, err := manager.ReadOutput(info.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if output.Content != "output via the seam\n" {
		t.Fatalf("task output = %q, want what the seam wrote", output.Content)
	}

	// Stopping must travel the same way; the manager owns no process handle of
	// its own to signal.
	if err := manager.Stop(info.ID); err != nil {
		t.Fatal(err)
	}
	if !procs[0].wasStopped() {
		t.Fatal("Stop did not reach the process returned by the seam")
	}

	deadline := time.After(2 * time.Second)
	for {
		states := manager.Snapshot()
		if len(states) == 1 && states[0].Status == TaskStopped {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("task did not reach stopped state: %+v", states)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
