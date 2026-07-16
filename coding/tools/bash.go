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
	Command string `json:"command" jsonschema:"description=The bash command to run,minLength=1"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds; defaults to 120,minimum=1"`
}

// Bash returns a tool that runs a shell command in the workspace directory and
// returns its combined output and exit code. A non-zero exit is reported to the
// model as content, not as a failure, so the model can react to it.
func Bash(root string, ops ExecOps) Tool {
	def := llm.MustTool[bashArgs]("bash", "Run a bash command in the workspace directory and return its combined output.")
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Bash",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in bashArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
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
		PromptSnippet: "bash: run a shell command in the workspace.",
		Guidelines: []string{
			"Prefer the dedicated read/edit/write tools over shell equivalents like cat or sed when one fits.",
		},
	}
}
