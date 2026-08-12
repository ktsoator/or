package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultConnectTimeout = 15 * time.Second

// Connection owns one initialized MCP client session and its discovered tools.
// Product-level managers can share it without depending on protocol internals.
type Connection struct {
	name       string
	transport  string
	tools      []Tool
	diagnostic string
	session    *protocol.ClientSession
	cancel     context.CancelFunc
	closeOnce  sync.Once
}

// Connect initializes one server and discovers its complete tools/list result.
// Workspace scoping is a product concern and must be checked by the caller.
func Connect(ctx context.Context, name string, config Config, workspace string) (*Connection, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	transport, err := buildTransport(config, workspace)
	if err != nil {
		return nil, err
	}
	timeout := defaultConnectTimeout
	if config.TimeoutSeconds > 0 {
		timeout = time.Duration(config.TimeoutSeconds) * time.Second
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := protocol.NewClient(&protocol.Implementation{Name: "or", Title: "Or", Version: "dev"}, &protocol.ClientOptions{
		Capabilities: &protocol.ClientCapabilities{},
	})
	connectionCtx, connectionCancel := context.WithCancel(ctx)
	type connectResult struct {
		session *protocol.ClientSession
		err     error
	}
	connected := make(chan connectResult, 1)
	go func() {
		session, err := client.Connect(connectionCtx, transport, nil)
		connected <- connectResult{session: session, err: err}
	}()
	var session *protocol.ClientSession
	select {
	case result := <-connected:
		session, err = result.session, result.err
	case <-connectCtx.Done():
		connectionCancel()
		go func() {
			result := <-connected
			if result.session != nil {
				_ = result.session.Close()
			}
		}()
		return nil, fmt.Errorf("connect %s: %w", name, connectCtx.Err())
	}
	if err != nil {
		connectionCancel()
		return nil, fmt.Errorf("connect %s: %w", name, err)
	}
	definitions, err := listTools(connectCtx, session)
	if err != nil {
		_ = session.Close()
		connectionCancel()
		return nil, fmt.Errorf("list tools from %s: %w", name, err)
	}

	connection := &Connection{
		name:      name,
		transport: transportName(config),
		session:   session,
		cancel:    connectionCancel,
	}
	for _, definition := range definitions {
		tool, err := newTool(name, connection.transport, definition, session)
		if err != nil {
			connection.diagnostic = appendDiagnostic(connection.diagnostic, err.Error())
			continue
		}
		connection.tools = append(connection.tools, tool)
	}
	return connection, nil
}

// Name returns the configured server name used for tool names and diagnostics.
func (connection *Connection) Name() string {
	if connection == nil {
		return ""
	}
	return connection.name
}

// Transport returns stdio or streamable_http.
func (connection *Connection) Transport() string {
	if connection == nil {
		return ""
	}
	return connection.transport
}

// Tools returns an independent slice of the tools discovered at initialization.
func (connection *Connection) Tools() []Tool {
	if connection == nil {
		return nil
	}
	return append([]Tool(nil), connection.tools...)
}

// Diagnostic reports non-fatal tool definition problems found at startup.
func (connection *Connection) Diagnostic() string {
	if connection == nil {
		return ""
	}
	return connection.diagnostic
}

// Close gracefully terminates the protocol session. It is idempotent.
func (connection *Connection) Close() {
	if connection == nil {
		return
	}
	connection.closeOnce.Do(func() {
		connection.cancel()
		_ = connection.session.Close()
	})
}

func listTools(ctx context.Context, session *protocol.ClientSession) ([]*protocol.Tool, error) {
	var definitions []*protocol.Tool
	cursor := ""
	for page := 0; page < 100; page++ {
		result, err := session.ListTools(ctx, &protocol.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, result.Tools...)
		if result.NextCursor == "" {
			return definitions, nil
		}
		cursor = result.NextCursor
	}
	return nil, fmt.Errorf("tool list exceeded 100 pages")
}

func appendDiagnostic(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}
