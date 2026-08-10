package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// atomicWriteFile writes through a same-directory temporary file and atomically
// renames it into place. Existing permissions and symlinks are preserved.
func atomicWriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := localWriteTarget(path)
	if err != nil {
		return err
	}

	mode := perm
	if info, statErr := os.Stat(target); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	temp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(mode); err != nil {
		return err
	}
	const chunkSize = 64 * 1024
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := data
		if len(chunk) > chunkSize {
			chunk = chunk[:chunkSize]
		}
		n, err := temp.Write(chunk)
		if err != nil {
			return err
		}
		if n != len(chunk) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	committed = true
	return nil
}

// localWriteTarget follows a final symlink chain so the atomic rename replaces
// the target file rather than the symlink itself. A dangling final target is
// returned so the write can create it when its parent exists.
func localWriteTarget(path string) (string, error) {
	current := filepath.Clean(path)
	seen := make(map[string]struct{})
	for {
		if _, ok := seen[current]; ok {
			return "", fmt.Errorf("write %s: symlink cycle", path)
		}
		seen[current] = struct{}{}

		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return current, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}

		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = filepath.Clean(target)
	}
}

type commandOutput struct {
	Output   string
	ExitCode int
}

// runCommand runs command with `bash -c` inside dir, returning combined output.
// A non-zero exit is returned in commandOutput with a nil error; only a failure to
// start the process is a Go error. ctx cancellation stops the command.
func runCommand(ctx context.Context, command string, dir string) (commandOutput, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir
	// Run in a dedicated process group and, on cancellation or timeout, kill the
	// whole group. The stock CommandContext cancel signals only `bash -c`, which
	// leaves grandchildren — the binary `go run` compiles and execs, a dev server,
	// npm's child — alive and holding their ports. WaitDelay bounds how long we
	// wait for the pipe to drain if a stray child keeps it open.
	configureProcessGroup(cmd)
	cmd.Cancel = func() error { return terminateProcessGroup(cmd, syscall.SIGKILL) }
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.CombinedOutput()
	result := commandOutput{Output: string(out)}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, err
	}
	return result, nil
}

// startCommand launches command with `bash -c` inside dir and returns once it is
// running, with its combined output going to out. The command leads its own
// process group so the whole tree it spawns can be stopped later.
func startCommand(command string, dir string, out io.Writer) (*localProcess, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	configureProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &localProcess{cmd: cmd}, nil
}

// localProcess is one background command running on the local machine.
type localProcess struct{ cmd *exec.Cmd }

func (p *localProcess) Wait() (int, error) {
	err := p.cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func (p *localProcess) Stop() error { return terminateProcessGroup(p.cmd, syscall.SIGTERM) }

func (p *localProcess) Kill() error { return terminateProcessGroup(p.cmd, syscall.SIGKILL) }
