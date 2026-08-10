// Package mcpclient connects the Or coding product to configured Model Context
// Protocol servers and projects their tools into the product's existing tool
// and permission contracts.
package mcpclient

import (
	"context"
	"errors"
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

	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultConnectTimeout = 15 * time.Second

type ServerState string

const (
	StateConnected  ServerState = "connected"
	StateDisabled   ServerState = "disabled"
	StateError      ServerState = "error"
	StateOutOfScope ServerState = "out_of_scope"
)

// ServerStatus is a secret-free connection diagnostic returned by mcp_status.
type ServerStatus struct {
	Name      string      `json:"name"`
	Transport string      `json:"transport,omitempty"`
	State     ServerState `json:"state"`
	ToolCount int         `json:"toolCount,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// Session owns the MCP connections and adapted tools for one coding session.
// Coding sessions are unloaded while idle, so their subprocesses and remote
// sessions naturally follow the same lifecycle.
type Session struct {
	tools       []tools.Tool
	statuses    []ServerStatus
	connections []*protocol.ClientSession
	cancels     []context.CancelFunc
	closeOnce   sync.Once
}

// Open loads path and connects every server visible to workspace. A missing
// config is the normal disabled state and returns nil. Invalid or unavailable
// servers are isolated as diagnostics so one integration cannot prevent a
// conversation from opening.
func Open(ctx context.Context, path, workspace string) *Session {
	servers, err := loadConfig(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	session := &Session{}
	if err != nil {
		session.statuses = []ServerStatus{{Name: "configuration", State: StateError, Error: err.Error()}}
		session.tools = []tools.Tool{session.statusTool()}
		return session
	}

	usedNames := map[string]struct{}{"mcp_status": {}}
	for _, server := range servers {
		status := ServerStatus{Name: server.name, Transport: transportName(server.config)}
		if server.config.Disabled {
			status.State = StateDisabled
			session.statuses = append(session.statuses, status)
			continue
		}
		if err := server.config.Validate(); err != nil {
			status.State = StateError
			status.Error = err.Error()
			session.statuses = append(session.statuses, status)
			continue
		}
		applies, err := server.config.AppliesTo(workspace)
		if err != nil {
			status.State = StateError
			status.Error = err.Error()
			session.statuses = append(session.statuses, status)
			continue
		}
		if !applies {
			status.State = StateOutOfScope
			session.statuses = append(session.statuses, status)
			continue
		}

		connection, cancel, definitions, err := connect(ctx, server.name, server.config, workspace)
		if err != nil {
			status.State = StateError
			status.Error = err.Error()
			session.statuses = append(session.statuses, status)
			continue
		}
		session.connections = append(session.connections, connection)
		session.cancels = append(session.cancels, cancel)
		access := permission.Network
		if strings.TrimSpace(server.config.Command) != "" {
			access = permission.Execute
		}
		for _, definition := range definitions {
			adapted, err := adaptTool(server.name, definition, connection, access)
			if err != nil {
				status.State = StateError
				status.Error = appendDiagnostic(status.Error, err.Error())
				continue
			}
			if _, duplicate := usedNames[adapted.Name()]; duplicate {
				status.State = StateError
				status.Error = appendDiagnostic(status.Error, fmt.Sprintf("tool name collision for %q", definition.Name))
				continue
			}
			usedNames[adapted.Name()] = struct{}{}
			session.tools = append(session.tools, adapted)
			status.ToolCount++
		}
		if status.State != StateError {
			status.State = StateConnected
		}
		session.statuses = append(session.statuses, status)
	}
	if len(servers) > 0 {
		session.tools = append(session.tools, session.statusTool())
	}
	return session
}

// Tools returns an independent slice of the tools visible to the coding agent.
func (session *Session) Tools() []tools.Tool {
	if session == nil {
		return nil
	}
	return append([]tools.Tool(nil), session.tools...)
}

// Close gracefully terminates every MCP connection. It is idempotent.
func (session *Session) Close() {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		for index := len(session.connections) - 1; index >= 0; index-- {
			session.cancels[index]()
			_ = session.connections[index].Close()
		}
	})
}

func connect(ctx context.Context, name string, config ServerConfig, workspace string) (*protocol.ClientSession, context.CancelFunc, []*protocol.Tool, error) {
	transport, err := buildTransport(config, workspace)
	if err != nil {
		return nil, nil, nil, err
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
	var connection *protocol.ClientSession
	select {
	case result := <-connected:
		connection, err = result.session, result.err
	case <-connectCtx.Done():
		connectionCancel()
		go func() {
			result := <-connected
			if result.session != nil {
				_ = result.session.Close()
			}
		}()
		return nil, nil, nil, fmt.Errorf("connect %s: %w", name, connectCtx.Err())
	}
	if err != nil {
		connectionCancel()
		return nil, nil, nil, fmt.Errorf("connect %s: %w", name, err)
	}
	definitions, err := listTools(connectCtx, connection)
	if err != nil {
		_ = connection.Close()
		connectionCancel()
		return nil, nil, nil, fmt.Errorf("list tools from %s: %w", name, err)
	}
	return connection, connectionCancel, definitions, nil
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
