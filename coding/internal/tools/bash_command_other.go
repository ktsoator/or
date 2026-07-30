//go:build !windows

package tools

import "os/exec"

func findBash() (string, error) {
	return exec.LookPath("bash")
}

func configureBashEnvironment(*exec.Cmd, string) {}
