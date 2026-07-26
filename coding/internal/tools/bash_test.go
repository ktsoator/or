package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/agent"
)

type stubExecOps struct {
	result ExecResult
	err    error
}

func (s stubExecOps) Exec(context.Context, string, string) (ExecResult, error) {
	return s.result, s.err
}

func TestBashReturnsExitCodeInToolOutcome(t *testing.T) {
	tests := []struct {
		name       string
		exitCode   int
		wantStatus agent.ToolOutcomeStatus
		wantCode   string
	}{
		{name: "zero exit", exitCode: 0, wantStatus: agent.ToolOutcomeSuccess},
		{name: "nonzero exit", exitCode: 7, wantStatus: agent.ToolOutcomeFailed, wantCode: "command_exit_nonzero"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Bash(t.TempDir(), stubExecOps{result: ExecResult{
				Output:   "command output",
				ExitCode: test.exitCode,
			}}, nil).Execute(
				context.Background(),
				"bash-call",
				json.RawMessage(`{"command":"test command"}`),
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
