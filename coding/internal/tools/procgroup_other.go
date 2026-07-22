//go:build !unix

package tools

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup is a no-op where process groups are unavailable.
func configureProcessGroup(*exec.Cmd) {}

// terminateProcessGroup falls back to killing the direct child, since there is
// no portable way to signal a whole process group here.
func terminateProcessGroup(cmd *exec.Cmd, _ syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
