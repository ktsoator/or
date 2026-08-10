package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// defaultBashTimeout bounds a single command when the model does not set one.
const defaultBashTimeout = 120 * time.Second

type bashArgs struct {
	Command         string `json:"command" jsonschema:"description=The bash command to run,minLength=1"`
	Description     string `json:"description,omitempty" jsonschema:"description=A short active-voice summary of what this command does (about 5-10 words), such as 'Install dependencies' or 'Run the test suite'. Shown in the UI in place of the raw command; always set it."`
	Timeout         int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds; defaults to 120,minimum=1"`
	RunInBackground bool   `json:"run_in_background,omitempty" jsonschema:"description=Run the command as a managed background task and return its task id and output file immediately. Completion is reported automatically. Read the returned output path for logs and use task_stop to stop it. Do not poll."`
}

// bashTool returns a tool that runs a shell command in the workspace directory and
// returns its combined output and exit code. A non-zero exit is a failed tool
// outcome that still preserves output for the model and the exact exit code for
// runtimes. When tasks is non-nil, run_in_background starts a managed task and
// returns its id and output path instead of blocking.
func bashTool(root string, tasks *TaskManager) Tool {
	def := llm.MustTool[bashArgs]("bash", bashText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Bash",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolProgress)) (agent.ToolResult, error) {
				var in bashArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}

				if in.RunInBackground {
					if tasks == nil {
						return failedResult("background_unavailable", "background execution is not available in this session", nil), nil
					}
					info, err := tasks.Start(in.Command, in.Description, root)
					if err != nil {
						return failedResult("command_start_failed", fmt.Sprintf("command failed to start: %v", err), nil), err
					}
					return textResult(fmt.Sprintf(
						"Started background task %s.\nOutput: %s\nCompletion will be reported automatically. Read the output file when logs are needed; stop it with task_stop(task_id=%q).",
						info.ID, info.OutputPath, info.ID,
					)), nil
				}

				timeout := defaultBashTimeout
				if in.Timeout > 0 {
					timeout = time.Duration(in.Timeout) * time.Second
				}
				runCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()

				result, err := runCommand(runCtx, in.Command, root)
				if err != nil {
					return failedResult("command_execution_failed", fmt.Sprintf("command failed to run: %v", err), nil), err
				}

				var b strings.Builder
				b.WriteString(truncate(result.Output, DefaultMaxLines, DefaultMaxBytes))
				if result.ExitCode != 0 {
					fmt.Fprintf(&b, "\n\n[exit code: %d]", result.ExitCode)
				}
				switch runCtx.Err() {
				case context.DeadlineExceeded:
					return commandResult(agent.ToolOutcomeTimeout, "command_timeout", b.String(), result.ExitCode), nil
				case context.Canceled:
					return commandResult(agent.ToolOutcomeCancelled, "command_cancelled", b.String(), result.ExitCode), nil
				}
				if result.ExitCode != 0 {
					return commandResult(agent.ToolOutcomeFailed, "command_exit_nonzero", b.String(), result.ExitCode), nil
				}
				return commandResult(agent.ToolOutcomeSuccess, "", b.String(), result.ExitCode), nil
			},
		},
		AccessFor:  commandAccess,
		Guidelines: bashText.guidelines,
	}
}
