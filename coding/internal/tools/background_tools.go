package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type taskStopArgs struct {
	TaskID string `json:"task_id" jsonschema:"description=The id of the background task to stop,minLength=1"`
}

// TaskStop returns a tool that stops a managed background task and its whole
// process group.
func TaskStop(tasks *TaskManager) Tool {
	def := llm.MustTool[taskStopArgs]("task_stop", taskStopText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Stop task",
			Execute: func(_ context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in taskStopArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				if err := tasks.Stop(in.TaskID); err != nil {
					return textResult(err.Error()), nil
				}
				return textResult(fmt.Sprintf("Stopped background task %s.", in.TaskID)), nil
			},
		},
		AccessFor: InternalAccess,
	}
}
