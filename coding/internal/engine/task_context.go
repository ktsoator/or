package engine

import (
	"fmt"
	"html"
	"strings"

	"github.com/ktsoator/or/coding/internal/tools"
)

const maxTaskStatusEntries = 20

func renderTaskStatus(completed []tools.TaskState) string {
	if len(completed) == 0 {
		return ""
	}
	if len(completed) > maxTaskStatusEntries {
		completed = completed[len(completed)-maxTaskStatusEntries:]
	}
	var rendered strings.Builder
	rendered.WriteString(`<or-context kind="task_status">`)
	rendered.WriteString("\nBackground task completion snapshot. Do not poll completed tasks; read an output file only when its logs are needed.\n")
	for _, task := range completed {
		exitCode := 0
		if task.ExitCode != nil {
			exitCode = *task.ExitCode
		}
		command := task.Command
		if len(command) > 300 {
			command = command[:300] + "..."
		}
		fmt.Fprintf(
			&rendered,
			`<task id="%s" status="%s" exit_code="%d" output_path="%s">%s</task>`+"\n",
			html.EscapeString(task.ID),
			html.EscapeString(string(task.Status)),
			exitCode,
			html.EscapeString(task.OutputPath),
			html.EscapeString(command),
		)
	}
	rendered.WriteString(`</or-context>`)
	return rendered.String()
}
