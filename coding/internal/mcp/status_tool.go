package mcp

import (
	"context"
	"encoding/json"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func statusTool(statuses []ServerStatus) tools.Tool {
	definition := llm.MustTool[struct{}]("mcp_status", "Report the configured MCP servers, their connection state, and the number of available tools.")
	return tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: definition,
			Label:      "MCP status",
			Execute: func(context.Context, string, json.RawMessage, func(agent.ToolProgress)) (agent.ToolResult, error) {
				encoded, err := json.MarshalIndent(statuses, "", "  ")
				if err != nil {
					return agent.ToolResult{}, err
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: string(encoded)}},
					Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess, Data: statuses},
				}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
}

func markUnavailable(statuses []ServerStatus, indexes map[string]int, serverName, diagnostic string) {
	index, ok := indexes[serverName]
	if !ok {
		return
	}
	status := &statuses[index]
	status.State = StateError
	if status.ToolCount > 0 {
		status.ToolCount--
	}
	status.Error = appendDiagnostic(status.Error, diagnostic)
}

func appendDiagnostic(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}
