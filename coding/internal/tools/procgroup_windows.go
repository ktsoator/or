//go:build windows

package tools

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func configureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup
	cmd.SysProcAttr.HideWindow = true
}

func terminateProcessGroup(cmd *exec.Cmd, _ syscall.Signal) error {
	if cmd.Process == nil || (cmd.ProcessState != nil && cmd.ProcessState.Exited()) {
		return nil
	}

	taskkill := "taskkill.exe"
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		candidate := filepath.Join(systemRoot, "System32", taskkill)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			taskkill = candidate
		}
	}
	kill := exec.Command(taskkill, "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := kill.CombinedOutput()
	if err == nil {
		return nil
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return nil
	}
	if directErr := cmd.Process.Kill(); errors.Is(directErr, os.ErrProcessDone) {
		return nil
	} else if directErr != nil {
		return fmt.Errorf("taskkill failed: %w (%s); direct kill failed: %v", err, strings.TrimSpace(string(output)), directErr)
	}
	return fmt.Errorf("taskkill failed, only the direct process was stopped: %w (%s)", err, strings.TrimSpace(string(output)))
}
