package mcpclient

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

func TestAdaptToolCallsMCPServerAndProjectsResult(t *testing.T) {
	ctx := context.Background()
	server := protocol.NewServer(&protocol.Implementation{Name: "test-server", Version: "1"}, nil)
	server.AddTool(&protocol.Tool{
		Name:        "echo.text",
		Description: "Echo text",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
	}, func(_ context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
		var input struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
			return nil, err
		}
		return &protocol.CallToolResult{
			Content:           []protocol.Content{&protocol.TextContent{Text: "echo: " + input.Text}},
			StructuredContent: map[string]any{"echoed": input.Text},
		}, nil
	})
	serverTransport, clientTransport := protocol.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := protocol.NewClient(&protocol.Implementation{Name: "test-client", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := adaptTool("demo server", listed.Tools[0], clientSession, permission.Network)
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
	result, err := tool.Execute(ctx, "call-1", json.RawMessage(`{"text":"hello"}`), func(agent.ToolProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Status != agent.ToolOutcomeSuccess {
		t.Fatalf("outcome = %#v", result.Outcome)
	}
	if len(result.Content) != 2 || result.Content[0].(*llm.TextContent).Text != "echo: hello" {
		t.Fatalf("content = %#v", result.Content)
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
	short := toolName("my server", "read.file")
	if short != "mcp__my_server__read_file" {
		t.Fatalf("short name = %q", short)
	}
	long := toolName(strings.Repeat("server", 20), strings.Repeat("tool", 20))
	if len(long) != maxProviderToolName {
		t.Fatalf("long name length = %d, want %d", len(long), maxProviderToolName)
	}
	if long != toolName(strings.Repeat("server", 20), strings.Repeat("tool", 20)) {
		t.Fatal("tool name is not stable")
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
