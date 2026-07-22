package tools

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// gracefulKillDelay is how long a background shell is given to exit after
// SIGTERM before it is force-killed with SIGKILL.
const gracefulKillDelay = 2 * time.Second

// BackgroundShells tracks shell commands started with run_in_background. Each
// runs in its own process group so its whole tree can be stopped at once, and
// its combined output is buffered for incremental reads via the bash_output
// tool. A shell keeps running across turns until it exits, is killed, or the
// session shuts down. The zero value is not usable; call NewBackgroundShells.
type BackgroundShells struct {
	mu      sync.Mutex
	counter int
	shells  map[string]*backgroundShell
}

// NewBackgroundShells returns an empty manager.
func NewBackgroundShells() *BackgroundShells {
	return &BackgroundShells{shells: map[string]*backgroundShell{}}
}

// BackgroundOutput is one bash_output poll: the output produced since the last
// poll plus the shell's current status.
type BackgroundOutput struct {
	ID       string
	Command  string
	Output   string // output accumulated since the previous poll
	Running  bool
	ExitCode int // meaningful only when Running is false
}

type backgroundShell struct {
	id      string
	command string
	cmd     *exec.Cmd
	done    chan struct{}

	mu       sync.Mutex
	buf      bytes.Buffer
	cursor   int // bytes already returned by Poll
	finished bool
	exitCode int
}

// shellWriter appends process output into the shell's buffer under its lock, so
// a concurrent Poll sees a consistent view.
type shellWriter struct{ sh *backgroundShell }

func (w shellWriter) Write(p []byte) (int, error) {
	w.sh.mu.Lock()
	defer w.sh.mu.Unlock()
	return w.sh.buf.Write(p)
}

// Start launches command in dir as a background shell and returns its id. The
// command is not waited on; its output accumulates for later Poll calls.
func (b *BackgroundShells) Start(command, dir string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = dir
	configureProcessGroup(cmd)

	sh := &backgroundShell{command: command, cmd: cmd, done: make(chan struct{})}
	w := shellWriter{sh: sh}
	cmd.Stdout = w
	cmd.Stderr = w

	if err := cmd.Start(); err != nil {
		return "", err
	}

	b.mu.Lock()
	b.counter++
	sh.id = "bg_" + strconv.Itoa(b.counter)
	b.shells[sh.id] = sh
	b.mu.Unlock()

	go func() {
		err := cmd.Wait()
		sh.mu.Lock()
		sh.finished = true
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			sh.exitCode = exitErr.ExitCode()
		} else if err != nil {
			sh.exitCode = -1
		}
		sh.mu.Unlock()
		close(sh.done)
	}()

	return sh.id, nil
}

// Poll returns the output produced since the previous Poll for the shell, along
// with its current status. It advances the read cursor, so repeated calls stream
// only new output.
func (b *BackgroundShells) Poll(id string) (BackgroundOutput, error) {
	sh, err := b.lookup(id)
	if err != nil {
		return BackgroundOutput{}, err
	}

	sh.mu.Lock()
	defer sh.mu.Unlock()
	all := sh.buf.Bytes()
	var fresh string
	if sh.cursor < len(all) {
		fresh = string(all[sh.cursor:])
		sh.cursor = len(all)
	}
	return BackgroundOutput{
		ID:       id,
		Command:  sh.command,
		Output:   fresh,
		Running:  !sh.finished,
		ExitCode: sh.exitCode,
	}, nil
}

// Kill stops a running background shell, terminating its whole process group. It
// is a no-op for a shell that has already exited.
func (b *BackgroundShells) Kill(id string) error {
	sh, err := b.lookup(id)
	if err != nil {
		return err
	}
	return terminate(sh)
}

// Shutdown stops every background shell. Call it when the owning session is torn
// down so long-lived processes do not outlive it.
func (b *BackgroundShells) Shutdown() {
	b.mu.Lock()
	shells := make([]*backgroundShell, 0, len(b.shells))
	for _, sh := range b.shells {
		shells = append(shells, sh)
	}
	b.mu.Unlock()
	for _, sh := range shells {
		_ = terminate(sh)
	}
}

func (b *BackgroundShells) lookup(id string) (*backgroundShell, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sh, ok := b.shells[id]
	if !ok {
		return nil, fmt.Errorf("no background shell %q", id)
	}
	return sh, nil
}

// terminate asks the shell's process group to stop, escalating from SIGTERM to
// SIGKILL if it does not exit promptly.
func terminate(sh *backgroundShell) error {
	sh.mu.Lock()
	finished := sh.finished
	sh.mu.Unlock()
	if finished {
		return nil
	}

	if err := terminateProcessGroup(sh.cmd, syscall.SIGTERM); err != nil {
		return err
	}
	select {
	case <-sh.done:
		return nil
	case <-time.After(gracefulKillDelay):
		return terminateProcessGroup(sh.cmd, syscall.SIGKILL)
	}
}
