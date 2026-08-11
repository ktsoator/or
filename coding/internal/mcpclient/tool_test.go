package mcpclient

import (
	"context"
	"encoding/json"
	"testing"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolKeepsProtocolDefinitionAndCallsServer(t *testing.T) {
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
			Content: []protocol.Content{&protocol.TextContent{Text: "echo: " + input.Text}},
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

	tool, err := newTool("demo", "stdio", listed.Tools[0], clientSession)
	if err != nil {
		t.Fatal(err)
	}
	if tool.ServerName() != "demo" || tool.Transport() != "stdio" || tool.Definition().Name != "echo.text" {
		t.Fatalf("tool metadata = %q, %q, %#v", tool.ServerName(), tool.Transport(), tool.Definition())
	}
	result, err := tool.Call(ctx, map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].(*protocol.TextContent).Text; got != "echo: hello" {
		t.Fatalf("content = %q", got)
	}
}

func TestNewToolRejectsMissingDefinitionAndName(t *testing.T) {
	for _, definition := range []*protocol.Tool{nil, {}} {
		if _, err := newTool("demo", "stdio", definition, nil); err == nil {
			t.Fatalf("newTool accepted definition %#v", definition)
		}
	}
}
