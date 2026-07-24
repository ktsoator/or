package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type bashOutputArgs struct {
	ShellID string `json:"shell_id" jsonschema:"description=The id returned when the command was started with run_in_background,minLength=1"`
}

// BashOutput returns a tool that reads new output from a background shell started
// by bash with run_in_background, reporting whether it is still running.
func BashOutput(shells *BackgroundShells) Tool {
	def := llm.MustTool[bashOutputArgs]("bash_output", bashOutputText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Background output",
			Execute: func(_ context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in bashOutputArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				out, err := shells.Poll(in.ShellID)
				if err != nil {
					return textResult(err.Error()), nil
				}
				return textResult(renderBackgroundOutput(out)), nil
			},
		},
		AccessFor: InternalAccess,
	}
}

type killBashArgs struct {
	ShellID string `json:"shell_id" jsonschema:"description=The id of the background shell to stop,minLength=1"`
}

// KillBash returns a tool that stops a background shell and its whole process
// group.
func KillBash(shells *BackgroundShells) Tool {
	def := llm.MustTool[killBashArgs]("kill_bash", killBashText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Kill background",
			Execute: func(_ context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in killBashArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				if err := shells.Kill(in.ShellID); err != nil {
					return textResult(err.Error()), nil
				}
				return textResult(fmt.Sprintf("Stopped background shell %s.", in.ShellID)), nil
			},
		},
		AccessFor: InternalAccess,
	}
}

func renderBackgroundOutput(out BackgroundOutput) string {
	var b strings.Builder
	if out.Running {
		fmt.Fprintf(&b, "[%s: running] %s\n", out.ID, out.Command)
	} else {
		fmt.Fprintf(&b, "[%s: exited with code %d] %s\n", out.ID, out.ExitCode, out.Command)
	}
	if out.Output == "" {
		b.WriteString("(no new output)")
	} else {
		b.WriteString(truncate(out.Output, DefaultMaxLines, DefaultMaxBytes))
	}
	return b.String()
}
