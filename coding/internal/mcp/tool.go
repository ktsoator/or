package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/mcp/client"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

type callableTool interface {
	ServerName() string
	Transport() string
	Definition() *protocol.Tool
	Call(context.Context, any) (*protocol.CallToolResult, error)
}

// buildTools adapts every discovered MCP tool for Or and appends the internal
// status tool when at least one server or configuration diagnostic exists.
func buildTools(discoveredTools []client.Tool, sourceStatuses []ServerStatus) []tools.Tool {
	statuses := append([]ServerStatus(nil), sourceStatuses...)
	statusIndex := make(map[string]int, len(statuses))
	for index := range statuses {
		statusIndex[statuses[index].Name] = index
	}

	usedNames := map[string]struct{}{"mcp_status": {}}
	adaptedTools := make([]tools.Tool, 0, len(discoveredTools)+1)
	for _, discovered := range discoveredTools {
		adapted, err := adaptTool(discovered)
		if err != nil {
			markUnavailable(statuses, statusIndex, discovered.ServerName(), err.Error())
			continue
		}
		if _, duplicate := usedNames[adapted.Name()]; duplicate {
			name := ""
			if definition := discovered.Definition(); definition != nil {
				name = definition.Name
			}
			markUnavailable(statuses, statusIndex, discovered.ServerName(), fmt.Sprintf("tool name collision for %q", name))
			continue
		}
		usedNames[adapted.Name()] = struct{}{}
		adaptedTools = append(adaptedTools, adapted)
	}
	if len(statuses) > 0 {
		adaptedTools = append(adaptedTools, statusTool(statuses))
	}
	return adaptedTools
}

func adaptTool(source callableTool) (tools.Tool, error) {
	definition := source.Definition()
	if definition == nil || strings.TrimSpace(definition.Name) == "" {
		return tools.Tool{}, fmt.Errorf("server returned a tool without a name")
	}
	parameters, err := json.Marshal(definition.InputSchema)
	if err != nil {
		return tools.Tool{}, fmt.Errorf("encode schema for %q: %w", definition.Name, err)
	}
	if definition.InputSchema == nil || string(parameters) == "null" {
		parameters = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	var schema map[string]any
	if err := json.Unmarshal(parameters, &schema); err != nil {
		return tools.Tool{}, fmt.Errorf("invalid schema for %q: %w", definition.Name, err)
	}
	if schema == nil {
		return tools.Tool{}, fmt.Errorf("schema for %q is not an object", definition.Name)
	}

	serverName := source.ServerName()
	advertisedName := ToolName(serverName, definition.Name)
	description := strings.TrimSpace(definition.Description)
	if description == "" {
		description = fmt.Sprintf("Call %s from the %s MCP server.", definition.Name, serverName)
	} else {
		description = fmt.Sprintf("MCP server %s: %s", serverName, description)
	}
	label := DisplayTitle(definition)
	access := permission.Network
	if source.Transport() == "stdio" {
		access = permission.Execute
	}
	return tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.ToolDefinition{
				Name:        advertisedName,
				Description: description,
				Parameters:  parameters,
			},
			Label: fmt.Sprintf("%s · %s", serverName, label),
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolProgress)) (agent.ToolResult, error) {
				var arguments map[string]any
				if err := json.Unmarshal(raw, &arguments); err != nil {
					return agent.ToolResult{}, err
				}
				result, err := source.Call(ctx, arguments)
				if err != nil {
					return agent.ToolResult{}, err
				}
				return projectResult(serverName, definition.Name, result), nil
			},
		},
		AccessFor: func(map[string]any) []permission.Access {
			return []permission.Access{{Action: access}}
		},
	}, nil
}
