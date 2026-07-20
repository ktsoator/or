//go:build unix

package tools

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup makes cmd the leader of a new process group so the whole
// tree it spawns can be signalled at once. Wrappers like `go run`, `npm`, and
// shells fork a child that keeps running after a kill aimed only at the direct
// child; signalling the group reaps them all and frees the port they held.
func configureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminateProcessGroup sends sig to every process in cmd's group. A negative
// pid targets the group led by the child. It is a no-op before the process
// starts, and treats an already-exited group as success.
func terminateProcessGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}
