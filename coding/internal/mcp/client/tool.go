package client

import (
	"context"
	"fmt"
	"strings"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool is one tool discovered from an MCP server. It retains the protocol
// definition and connection needed to call the original server without
// depending on Or's agent, provider, or permission types.
type Tool struct {
	serverName string
	transport  string
	definition *protocol.Tool
	session    *protocol.ClientSession
}

func newTool(serverName, transport string, definition *protocol.Tool, session *protocol.ClientSession) (Tool, error) {
	if definition == nil || strings.TrimSpace(definition.Name) == "" {
		return Tool{}, fmt.Errorf("server returned a tool without a name")
	}
	return Tool{
		serverName: serverName,
		transport:  transport,
		definition: definition,
		session:    session,
	}, nil
}

// ServerName returns the configured name of the server that owns the tool.
func (tool Tool) ServerName() string { return tool.serverName }

// Transport returns the MCP transport used by the owning server.
func (tool Tool) Transport() string { return tool.transport }

// Definition returns the MCP protocol definition advertised by the server.
// Callers must treat the returned definition as read-only.
func (tool Tool) Definition() *protocol.Tool { return tool.definition }

// Call invokes the tool with protocol-native arguments and returns the
// protocol-native result.
func (tool Tool) Call(ctx context.Context, arguments any) (*protocol.CallToolResult, error) {
	if tool.session == nil || tool.definition == nil {
		return nil, fmt.Errorf("MCP tool is not connected")
	}
	result, err := tool.session.CallTool(ctx, &protocol.CallToolParams{
		Name:      tool.definition.Name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("MCP tool %s/%s: %w", tool.serverName, tool.definition.Name, err)
	}
	return result, nil
}
