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

// DefaultBashTimeout bounds a single command when the model does not set one.
const DefaultBashTimeout = 120 * time.Second

type bashArgs struct {
	Command         string `json:"command" jsonschema:"description=The bash command to run,minLength=1"`
	Description     string `json:"description,omitempty" jsonschema:"description=A short active-voice summary of what this command does (about 5-10 words), such as 'Install dependencies' or 'Run the test suite'. Shown in the UI in place of the raw command; always set it."`
	Timeout         int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds; defaults to 120,minimum=1"`
	RunInBackground bool   `json:"run_in_background,omitempty" jsonschema:"description=Run the command in the background and return immediately with a shell id, instead of waiting for it to exit. Use this for long-lived processes such as dev servers. Read its output later with bash_output and stop it with kill_bash."`
}

// Bash returns a tool that runs a shell command in the workspace directory and
// returns its combined output and exit code. A non-zero exit is reported to the
// model as content, not as a failure, so the model can react to it. When shells
// is non-nil, run_in_background starts long-lived commands detached and returns a
// shell id instead of blocking.
func Bash(root string, ops ExecOps, shells *BackgroundShells) Tool {
	def := llm.MustTool[bashArgs]("bash", bashText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Bash",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in bashArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}

				if in.RunInBackground {
					if shells == nil {
						return textResult("background execution is not available in this session"), nil
					}
					id, err := shells.Start(in.Command, root)
					if err != nil {
						return textResult(fmt.Sprintf("command failed to start: %v", err)), err
					}
					return textResult(fmt.Sprintf(
						"Started background shell %s.\nRead new output with bash_output(shell_id=%q); stop it with kill_bash(shell_id=%q).",
						id, id, id,
					)), nil
				}

				timeout := DefaultBashTimeout
				if in.Timeout > 0 {
					timeout = time.Duration(in.Timeout) * time.Second
				}
				runCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()

				result, err := ops.Exec(runCtx, in.Command, root)
				if err != nil {
					return textResult(fmt.Sprintf("command failed to run: %v", err)), err
				}

				var b strings.Builder
				b.WriteString(truncate(result.Output, DefaultMaxLines, DefaultMaxBytes))
				if result.ExitCode != 0 {
					fmt.Fprintf(&b, "\n\n[exit code: %d]", result.ExitCode)
				}
				return textResult(b.String()), nil
			},
		},
		// A bash call is read-only only when its command merely inspects state, so
		// inspection commands run without confirmation while anything that could
		// change the workspace still asks.
		ReadOnlyFor: func(args map[string]any) bool {
			command, _ := args["command"].(string)
			return commandIsReadOnly(command)
		},
		PromptSnippet: bashText.snippet,
		Guidelines:    bashText.guidelines,
	}
}
