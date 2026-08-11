package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
func Connect(ctx context.Context, name string, config ServerConfig, workspace string) (*Connection, error) {
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

func buildTransport(config ServerConfig, workspace string) (protocol.Transport, error) {
	if strings.TrimSpace(config.Command) != "" {
		command, err := expand(config.Command, workspace)
		if err != nil {
			return nil, err
		}
		args := make([]string, len(config.Args))
		for index, arg := range config.Args {
			args[index], err = expand(arg, workspace)
			if err != nil {
				return nil, err
			}
		}
		cmd := exec.Command(command, args...)
		cwd := workspace
		if strings.TrimSpace(config.Cwd) != "" {
			cwd, err = expand(config.Cwd, workspace)
			if err != nil {
				return nil, err
			}
			cwd, err = expandHome(cwd)
			if err != nil {
				return nil, err
			}
		}
		if !filepath.IsAbs(cwd) {
			cwd = filepath.Join(workspace, cwd)
		}
		cmd.Dir = filepath.Clean(cwd)
		cmd.Env, err = mergedEnvironment(config.Env, workspace)
		if err != nil {
			return nil, err
		}
		return &protocol.CommandTransport{Command: cmd}, nil
	}

	endpoint, err := expand(config.URL, workspace)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid Streamable HTTP URL %q", endpoint)
	}
	headers := make(http.Header, len(config.Headers))
	for key, value := range config.Headers {
		expanded, err := expand(value, workspace)
		if err != nil {
			return nil, err
		}
		headers.Set(key, expanded)
	}
	client := &http.Client{Transport: &headerTransport{base: http.DefaultTransport, origin: parsed.Scheme + "://" + parsed.Host, headers: headers}}
	return &protocol.StreamableClientTransport{Endpoint: endpoint, HTTPClient: client}, nil
}

func mergedEnvironment(configured map[string]string, workspace string) ([]string, error) {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		if key, value, found := strings.Cut(entry, "="); found {
			if inheritedMCPEnvironment[strings.ToUpper(key)] {
				values[key] = value
			}
		}
	}
	for key, value := range configured {
		expanded, err := expand(value, workspace)
		if err != nil {
			return nil, err
		}
		values[key] = expanded
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment, nil
}

// MCP commands are executable configuration, but they should not receive every
// credential carried by the desktop sidecar. Servers opt into additional
// values through their explicit env map and ${env:NAME} references.
var inheritedMCPEnvironment = map[string]bool{
	"APPDATA": true, "COLORTERM": true, "COMSPEC": true,
	"HOME": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"LOCALAPPDATA": true, "LOGNAME": true, "PATH": true, "PATHEXT": true,
	"SHELL": true, "SYSTEMROOT": true, "TEMP": true, "TERM": true,
	"TMP": true, "TMPDIR": true, "USER": true, "USERPROFILE": true,
	"WINDIR": true, "XDG_CACHE_HOME": true, "XDG_CONFIG_HOME": true,
	"XDG_DATA_HOME": true,
}

type headerTransport struct {
	base    http.RoundTripper
	origin  string
	headers http.Header
}

func (transport *headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	if clone.URL.Scheme+"://"+clone.URL.Host == transport.origin {
		for key, values := range transport.headers {
			clone.Header.Del(key)
			for _, value := range values {
				clone.Header.Add(key, value)
			}
		}
	}
	return transport.base.RoundTrip(clone)
}

func transportName(config ServerConfig) string {
	if strings.TrimSpace(config.Command) != "" {
		return "stdio"
	}
	if strings.TrimSpace(config.URL) != "" {
		return "streamable_http"
	}
	return ""
}

func appendDiagnostic(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}
