// Package mcpbridge adapts protocol-native MCP tools and results to Or's
// coding-agent, model-provider, and permission contracts.
package mcpbridge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/mcpclient"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxProviderToolName = 64

const (
	maxResultTextBytes  = 30_000
	maxResultImageBytes = 20 << 20
)

type callableTool interface {
	ServerName() string
	Transport() string
	Definition() *protocol.Tool
	Call(context.Context, any) (*protocol.CallToolResult, error)
}

// BuildTools adapts every discovered MCP tool for Or and appends the internal
// status tool when at least one server or configuration diagnostic exists.
func BuildTools(session *mcpclient.Session) []tools.Tool {
	if session == nil {
		return nil
	}
	statuses := session.Statuses()
	statusIndex := make(map[string]int, len(statuses))
	for index := range statuses {
		statusIndex[statuses[index].Name] = index
	}

	usedNames := map[string]struct{}{"mcp_status": {}}
	adaptedTools := make([]tools.Tool, 0, len(session.Tools())+1)
	for _, discovered := range session.Tools() {
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

// DisplayTitle returns the MCP-standard display-name precedence for a tool.
func DisplayTitle(definition *protocol.Tool) string {
	if definition == nil {
		return ""
	}
	if title := strings.TrimSpace(definition.Title); title != "" {
		return title
	}
	if definition.Annotations != nil {
		if title := strings.TrimSpace(definition.Annotations.Title); title != "" {
			return title
		}
	}
	return definition.Name
}

func projectResult(serverName, toolName string, result *protocol.CallToolResult) agent.ToolResult {
	if result == nil {
		return agent.ToolResult{
			Content: []llm.ToolResultContent{&llm.TextContent{Text: "MCP server returned an empty result"}},
			Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeFailed, ErrorCode: "mcp_empty_result"},
		}
	}
	content := make([]llm.ToolResultContent, 0, len(result.Content)+1)
	remainingText := maxResultTextBytes
	remainingImages := maxResultImageBytes
	appendText := func(text string) {
		if remainingText <= 0 {
			return
		}
		if len(text) > remainingText {
			text = validPrefix(text, remainingText) + fmt.Sprintf("\n\n[truncated: MCP output exceeded %d bytes]", maxResultTextBytes)
			remainingText = 0
		} else {
			remainingText -= len(text)
		}
		content = append(content, &llm.TextContent{Text: text})
	}
	for _, block := range result.Content {
		switch value := block.(type) {
		case *protocol.TextContent:
			appendText(value.Text)
		case *protocol.ImageContent:
			if len(value.Data) > remainingImages {
				appendText(fmt.Sprintf("[image omitted: MCP image output exceeded %d bytes]", maxResultImageBytes))
				continue
			}
			remainingImages -= len(value.Data)
			content = append(content, &llm.ImageContent{
				Data:     base64.StdEncoding.EncodeToString(value.Data),
				MIMEType: value.MIMEType,
			})
		default:
			encoded, err := json.Marshal(value)
			if err == nil {
				appendText(string(encoded))
			}
		}
	}
	if result.StructuredContent != nil {
		if encoded, err := json.MarshalIndent(result.StructuredContent, "", "  "); err == nil {
			appendText("Structured content:\n" + string(encoded))
		}
	}
	if result.NeedsInput() {
		content = append(content, &llm.TextContent{Text: "This MCP tool requires interactive input, which Or does not support yet."})
		return agent.ToolResult{
			Content: content,
			Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeFailed, ErrorCode: "mcp_input_required"},
		}
	}
	if len(content) == 0 {
		content = append(content, &llm.TextContent{Text: "MCP tool completed without content"})
	}
	outcome := agent.ToolOutcome{
		Status: agent.ToolOutcomeSuccess,
		Data: map[string]any{
			"server": serverName,
			"tool":   toolName,
		},
	}
	if result.IsError {
		outcome.Status = agent.ToolOutcomeFailed
		outcome.ErrorCode = "mcp_tool_error"
	}
	return agent.ToolResult{Content: content, Outcome: outcome}
}

func validPrefix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && value[maxBytes]&0xc0 == 0x80 {
		maxBytes--
	}
	return value[:maxBytes]
}

func statusTool(statuses []mcpclient.ServerStatus) tools.Tool {
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

func markUnavailable(statuses []mcpclient.ServerStatus, indexes map[string]int, serverName, diagnostic string) {
	index, ok := indexes[serverName]
	if !ok {
		return
	}
	status := &statuses[index]
	status.State = mcpclient.StateError
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

// ToolName returns the provider-safe, deterministic name Or advertises for an
// MCP tool.
func ToolName(serverName, originalName string) string {
	raw := "mcp__" + sanitizeName(serverName) + "__" + sanitizeName(originalName)
	if len(raw) <= maxProviderToolName {
		return raw
	}
	hash := sha256.Sum256([]byte(raw))
	suffix := "_" + hex.EncodeToString(hash[:4])
	return raw[:maxProviderToolName-len(suffix)] + suffix
}

func sanitizeName(value string) string {
	var result strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if char <= unicode.MaxASCII && (unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '-') {
			result.WriteRune(char)
		} else {
			result.WriteByte('_')
		}
	}
	if result.Len() == 0 {
		return "unnamed"
	}
	return result.String()
}
