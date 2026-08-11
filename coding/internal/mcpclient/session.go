// Package mcpclient manages configured Model Context Protocol connections and
// exposes protocol-native tool definitions and results to product adapters.
package mcpclient

import (
	"context"
	"errors"
	"os"
	"sync"
)

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
// It remains as the compatibility aggregate used while connection ownership is
// moved to the application-level manager.
type Session struct {
	tools       []Tool
	statuses    []ServerStatus
	connections []*Connection
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
		return session
	}

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

		connection, err := Connect(ctx, server.name, server.config, workspace)
		if err != nil {
			status.State = StateError
			status.Error = err.Error()
			session.statuses = append(session.statuses, status)
			continue
		}
		session.connections = append(session.connections, connection)
		discovered := connection.Tools()
		session.tools = append(session.tools, discovered...)
		status.ToolCount = len(discovered)
		if diagnostic := connection.Diagnostic(); diagnostic != "" {
			status.State = StateError
			status.Error = diagnostic
		} else {
			status.State = StateConnected
		}
		session.statuses = append(session.statuses, status)
	}
	return session
}

// Tools returns an independent slice of the tools discovered from connected
// servers.
func (session *Session) Tools() []Tool {
	if session == nil {
		return nil
	}
	return append([]Tool(nil), session.tools...)
}

// Statuses returns secret-free connection diagnostics for all configured
// servers.
func (session *Session) Statuses() []ServerStatus {
	if session == nil {
		return nil
	}
	return append([]ServerStatus(nil), session.statuses...)
}

// Close gracefully terminates every MCP connection. It is idempotent.
func (session *Session) Close() {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		for index := len(session.connections) - 1; index >= 0; index-- {
			session.connections[index].Close()
		}
	})
}
