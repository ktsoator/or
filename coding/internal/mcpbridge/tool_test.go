package mcpbridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeTool struct {
	server     string
	transport  string
	definition *protocol.Tool
	result     *protocol.CallToolResult
	arguments  any
}

func (tool *fakeTool) ServerName() string         { return tool.server }
func (tool *fakeTool) Transport() string          { return tool.transport }
func (tool *fakeTool) Definition() *protocol.Tool { return tool.definition }
func (tool *fakeTool) Call(_ context.Context, arguments any) (*protocol.CallToolResult, error) {
	tool.arguments = arguments
	return tool.result, nil
}

func TestAdaptToolCallsMCPClientAndProjectsResult(t *testing.T) {
	source := &fakeTool{
		server:    "demo server",
		transport: "streamable_http",
		definition: &protocol.Tool{
			Name:        "echo.text",
			Description: "Echo text",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		},
		result: &protocol.CallToolResult{
			Content:           []protocol.Content{&protocol.TextContent{Text: "echo: hello"}},
			StructuredContent: map[string]any{"echoed": "hello"},
		},
	}
	tool, err := adaptTool(source)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name() != "mcp__demo_server__echo_text" {
		t.Fatalf("tool name = %q", tool.Name())
	}
	accesses := tool.Accesses(map[string]any{"text": "hello"})
	if len(accesses) != 1 || accesses[0].Action != permission.Network {
		t.Fatalf("accesses = %#v", accesses)
	}
	result, err := tool.Execute(context.Background(), "call-1", json.RawMessage(`{"text":"hello"}`), func(agent.ToolProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if arguments := source.arguments.(map[string]any); arguments["text"] != "hello" {
		t.Fatalf("arguments = %#v", arguments)
	}
	if result.Outcome.Status != agent.ToolOutcomeSuccess {
		t.Fatalf("outcome = %#v", result.Outcome)
	}
	if len(result.Content) != 2 || result.Content[0].(*llm.TextContent).Text != "echo: hello" {
		t.Fatalf("content = %#v", result.Content)
	}
}

func TestAdaptToolMapsStdioToExecuteAccess(t *testing.T) {
	tool, err := adaptTool(&fakeTool{
		server: "local", transport: "stdio",
		definition: &protocol.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	accesses := tool.Accesses(nil)
	if len(accesses) != 1 || accesses[0].Action != permission.Execute {
		t.Fatalf("accesses = %#v", accesses)
	}
}

func TestProjectResultPreservesMCPToolErrors(t *testing.T) {
	result := projectResult("server", "tool", &protocol.CallToolResult{
		Content: []protocol.Content{&protocol.TextContent{Text: "invalid request"}},
		IsError: true,
	})
	if result.Outcome.Status != agent.ToolOutcomeFailed || result.Outcome.ErrorCode != "mcp_tool_error" {
		t.Fatalf("outcome = %#v", result.Outcome)
	}
	if got := result.Content[0].(*llm.TextContent).Text; got != "invalid request" {
		t.Fatalf("content = %q", got)
	}
}

func TestToolNameIsProviderSafeAndStable(t *testing.T) {
	short := ToolName("my server", "read.file")
	if short != "mcp__my_server__read_file" {
		t.Fatalf("short name = %q", short)
	}
	long := ToolName(strings.Repeat("server", 20), strings.Repeat("tool", 20))
	if len(long) != maxProviderToolName {
		t.Fatalf("long name length = %d, want %d", len(long), maxProviderToolName)
	}
	if long != ToolName(strings.Repeat("server", 20), strings.Repeat("tool", 20)) {
		t.Fatal("tool name is not stable")
	}
}

func TestDisplayTitleUsesProtocolPrecedence(t *testing.T) {
	definition := &protocol.Tool{
		Name:        "raw-name",
		Title:       "Display title",
		Annotations: &protocol.ToolAnnotations{Title: "Annotation title"},
	}
	if got := DisplayTitle(definition); got != "Display title" {
		t.Fatalf("display title = %q", got)
	}
	definition.Title = ""
	if got := DisplayTitle(definition); got != "Annotation title" {
		t.Fatalf("annotation title = %q", got)
	}
}

func TestProjectResultCapsTextOutput(t *testing.T) {
	result := projectResult("server", "tool", &protocol.CallToolResult{
		Content: []protocol.Content{&protocol.TextContent{Text: strings.Repeat("x", maxResultTextBytes+100)}},
	})
	text := result.Content[0].(*llm.TextContent).Text
	if !strings.Contains(text, "[truncated: MCP output exceeded") {
		t.Fatalf("output was not marked as truncated: %d bytes", len(text))
	}
}
