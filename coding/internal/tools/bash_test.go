package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/agent"
)

func TestBashReturnsExitCodeInToolOutcome(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		exitCode   int
		wantStatus agent.ToolOutcomeStatus
		wantCode   string
	}{
		{name: "zero exit", command: "printf 'command output'", exitCode: 0, wantStatus: agent.ToolOutcomeSuccess},
		{name: "nonzero exit", command: "printf 'command output'; exit 7", exitCode: 7, wantStatus: agent.ToolOutcomeFailed, wantCode: "command_exit_nonzero"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args, err := json.Marshal(bashArgs{Command: test.command})
			if err != nil {
				t.Fatal(err)
			}
			result, err := bashTool(t.TempDir(), nil).Execute(
				context.Background(),
				"bash-call",
				args,
				func(agent.ToolProgress) {},
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome.Status != test.wantStatus || result.Outcome.ErrorCode != test.wantCode {
				t.Fatalf("outcome = %#v, want status=%q code=%q", result.Outcome, test.wantStatus, test.wantCode)
			}
			if result.Outcome.ExitCode == nil || *result.Outcome.ExitCode != test.exitCode {
				t.Fatalf("exit code = %#v, want %d", result.Outcome.ExitCode, test.exitCode)
			}
		})
	}
}
