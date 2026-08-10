package mcpclient

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

func adaptTool(serverName string, definition *protocol.Tool, session *protocol.ClientSession, access permission.Action) (tools.Tool, error) {
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

	advertisedName := toolName(serverName, definition.Name)
	description := strings.TrimSpace(definition.Description)
	if description == "" {
		description = fmt.Sprintf("Call %s from the %s MCP server.", definition.Name, serverName)
	} else {
		description = fmt.Sprintf("MCP server %s: %s", serverName, description)
	}
	label := definition.Name
	if definition.Annotations != nil && strings.TrimSpace(definition.Annotations.Title) != "" {
		label = definition.Annotations.Title
	}
	originalName := definition.Name
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
				result, err := session.CallTool(ctx, &protocol.CallToolParams{Name: originalName, Arguments: arguments})
				if err != nil {
					return agent.ToolResult{}, fmt.Errorf("MCP tool %s/%s: %w", serverName, originalName, err)
				}
				return projectResult(serverName, originalName, result), nil
			},
		},
		AccessFor: func(map[string]any) []permission.Access {
			return []permission.Access{{Action: access}}
		},
	}, nil
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

func (session *Session) statusTool() tools.Tool {
	definition := llm.MustTool[struct{}]("mcp_status", "Report the configured MCP servers, their connection state, and the number of available tools.")
	return tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: definition,
			Label:      "MCP status",
			Execute: func(context.Context, string, json.RawMessage, func(agent.ToolProgress)) (agent.ToolResult, error) {
				encoded, err := json.MarshalIndent(session.statuses, "", "  ")
				if err != nil {
					return agent.ToolResult{}, err
				}
				return agent.ToolResult{
					Content: []llm.ToolResultContent{&llm.TextContent{Text: string(encoded)}},
					Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess, Data: session.statuses},
				}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
}

func toolName(serverName, originalName string) string {
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
