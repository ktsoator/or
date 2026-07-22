// Package tools implements the coding agent's built-in tools: file reading,
// editing, writing, and shell execution. Each tool is a definition-first Tool
// that carries both its executable body and the metadata it contributes to the
// system prompt, so the prompt is assembled from whichever tools are active.
//
// Filesystem and command execution go through the FileOps and ExecOps seams
// rather than touching os/exec directly. LocalOps is the default, running
// against the local filesystem and shell; override the seams to sandbox,
// containerize, or drive a remote workspace.
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

// FileOps abstracts filesystem access for the file tools. Paths passed to it are
// already resolved to absolute form by the tool. Implementations must honor ctx
// cancellation where the underlying operation supports it.
type FileOps interface {
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	// WriteFile replaces path without exposing a partially written destination
	// when the backend supports atomic replacement.
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	Stat(ctx context.Context, path string) (os.FileInfo, error)
	ReadDir(ctx context.Context, path string) ([]os.DirEntry, error)
}

// ExecOps abstracts shell command execution for the bash tool.
type ExecOps interface {
	// Exec runs command in a shell within dir and returns its combined output.
	// A non-zero exit code is reported in ExecResult, not as an error; an error
	// is returned only when the command could not be started. Exec must honor ctx
	// cancellation (e.g. a timeout).
	Exec(ctx context.Context, command string, dir string) (ExecResult, error)
}

// ExecResult is the outcome of one shell command.
type ExecResult struct {
	// Output is the combined stdout and stderr.
	Output string
	// ExitCode is the process exit status. Zero means success.
	ExitCode int
}

// Ops is the full operation surface the built-in tools need. LocalOps satisfies
// it; a custom backend can compose its own value from a FileOps and an ExecOps.
type Ops interface {
	FileOps
	ExecOps
}

// LocalOps runs against the local filesystem and a bash shell. It is the default
// backend and holds no state.
type LocalOps struct{}

// Open opens path for streaming reads.
func (LocalOps) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.Open(path)
}

// ReadFile reads the file at path.
func (LocalOps) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes through a same-directory temporary file and atomically
// renames it into place. Existing permissions and symlinks are preserved.
func (LocalOps) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
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

// MkdirAll creates path and any missing parents.
func (LocalOps) MkdirAll(ctx context.Context, path string, perm os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

// Stat returns file info for path.
func (LocalOps) Stat(_ context.Context, path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// ReadDir lists the directory entries of path.
func (LocalOps) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
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

// Exec runs command with `bash -c` inside dir, returning combined output. A
// non-zero exit is returned in ExecResult with a nil error; only a failure to
// start the process is a Go error. ctx cancellation stops the command.
func (LocalOps) Exec(ctx context.Context, command string, dir string) (ExecResult, error) {
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
	result := ExecResult{Output: string(out)}
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
