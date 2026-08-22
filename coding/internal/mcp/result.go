package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/imageprep"
	"github.com/ktsoator/or/llm"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxResultTextBytes  = 30_000
	maxResultImageBytes = 20 << 20
)

func projectResult(ctx context.Context, serverName, toolName string, result *protocol.CallToolResult) (agent.ToolResult, error) {
	if result == nil {
		return agent.ToolResult{
			Content: []llm.ToolResultContent{&llm.TextContent{Text: "MCP server returned an empty result"}},
			Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeFailed, ErrorCode: "mcp_empty_result"},
		}, nil
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
			if value == nil {
				continue
			}
			appendText(value.Text)
		case *protocol.ImageContent:
			if value == nil {
				appendText("[image omitted: MCP server returned empty image content]")
				continue
			}
			if len(value.Data) > remainingImages {
				appendText(fmt.Sprintf("[image omitted: MCP image output exceeded %d bytes]", maxResultImageBytes))
				continue
			}
			remainingImages -= len(value.Data)
			prepared, err := imageprep.Prepare(ctx, imageprep.Input{
				Data:         value.Data,
				DeclaredMIME: value.MIMEType,
			}, imageprep.DefaultPolicy())
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return agent.ToolResult{}, err
				}
				appendText(fmt.Sprintf("[image omitted: %v]", err))
				continue
			}
			content = append(content, &prepared.Content)
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
		}, nil
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
	return agent.ToolResult{Content: content, Outcome: outcome}, nil
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
