package tools

import (
	"context"
	"os/exec"
)

// ValidateLocalShell checks that the Bash backend required by Coding can start.
func ValidateLocalShell() error {
	_, err := findBash()
	return err
}

func newBashCommand(command, dir string) (*exec.Cmd, error) {
	executable, err := findBash()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, "-c", command)
	prepareBashCommand(cmd, dir)
	return cmd, nil
}

func newBashCommandContext(ctx context.Context, command, dir string) (*exec.Cmd, error) {
	executable, err := findBash()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, executable, "-c", command)
	prepareBashCommand(cmd, dir)
	return cmd, nil
}

func prepareBashCommand(cmd *exec.Cmd, dir string) {
	cmd.Dir = dir
	configureBashEnvironment(cmd, dir)
	configureProcessGroup(cmd)
}
