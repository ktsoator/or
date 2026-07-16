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
	"os"
	"os/exec"
)

// FileOps abstracts filesystem access for the file tools. Paths passed to it are
// already resolved to absolute form by the tool. Implementations must honor ctx
// cancellation where the underlying operation supports it.
type FileOps interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	Stat(ctx context.Context, path string) (os.FileInfo, error)
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

// ReadFile reads the file at path.
func (LocalOps) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes data to path with the given permissions, truncating any
// existing file.
func (LocalOps) WriteFile(_ context.Context, path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// MkdirAll creates path and any missing parents.
func (LocalOps) MkdirAll(_ context.Context, path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Stat returns file info for path.
func (LocalOps) Stat(_ context.Context, path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// Exec runs command with `bash -c` inside dir, returning combined output. A
// non-zero exit is returned in ExecResult with a nil error; only a failure to
// start the process is a Go error. ctx cancellation stops the command.
func (LocalOps) Exec(ctx context.Context, command string, dir string) (ExecResult, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir
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
