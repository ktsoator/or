package mcpmanager

import (
	"context"
	"errors"
	"sort"

	"github.com/ktsoator/or/coding/internal/mcpclient"
)

// Acquire waits for every applicable server in the current generation and
// returns one immutable snapshot. Individual server failures are represented
// in Statuses and never fail the whole lease.
func (manager *Manager) Acquire(ctx context.Context, workspace string) *Lease {
	return manager.acquire(ctx, workspace, false)
}

func (manager *Manager) acquire(ctx context.Context, workspace string, globalOnly bool) *Lease {
	_ = manager.reloadIfChanged()
	workspace, workspaceErr := normalizeWorkspace(workspace)
	config, generation, closed, configErr := manager.snapshotConfig()
	lease := &Lease{manager: manager}
	if closed {
		lease.statuses = []mcpclient.ServerStatus{{Name: "configuration", State: mcpclient.StateError, Error: "MCP manager is closed"}}
		return lease
	}
	if workspaceErr != nil {
		lease.statuses = []mcpclient.ServerStatus{{Name: "configuration", State: mcpclient.StateError, Error: workspaceErr.Error()}}
		return lease
	}
	if configErr != nil {
		lease.statuses = []mcpclient.ServerStatus{{Name: "configuration", State: mcpclient.StateError, Error: configErr.Error()}}
		return lease
	}

	names := make([]string, 0, len(config.MCPServers))
	for name := range config.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	type resolution struct {
		index      int
		status     mcpclient.ServerStatus
		connection managedConnection
		entry      *connectionEntry
		err        error
	}
	resolved := make(chan resolution, len(names))
	pending := 0
	lease.statuses = make([]mcpclient.ServerStatus, len(names))
	for index, name := range names {
		config := config.MCPServers[name]
		status := mcpclient.ServerStatus{Name: name, Transport: transportName(config)}
		if config.Disabled {
			status.State = mcpclient.StateDisabled
			lease.statuses[index] = status
			continue
		}
		if err := config.Validate(); err != nil {
			status.State = mcpclient.StateError
			status.Error = err.Error()
			lease.statuses[index] = status
			continue
		}
		if globalOnly && (len(config.Workspaces) > 0 || connectionScope(config, workspace) != "global") {
			continue
		}
		applies, err := config.AppliesTo(workspace)
		if err != nil {
			status.State = mcpclient.StateError
			status.Error = err.Error()
			lease.statuses[index] = status
			continue
		}
		if !applies {
			status.State = mcpclient.StateOutOfScope
			lease.statuses[index] = status
			continue
		}
		pending++
		go func(index int, name string, config mcpclient.ServerConfig, status mcpclient.ServerStatus) {
			key := connectionKey{generation: generation, server: name, scope: connectionScope(config, workspace)}
			connection, entry, err := manager.acquireConnection(ctx, key, config, workspace)
			if err != nil {
				status.State = mcpclient.StateError
				status.Error = err.Error()
				resolved <- resolution{index: index, status: status, err: err}
				return
			}
			status.ToolCount = len(connection.Tools())
			if diagnostic := connection.Diagnostic(); diagnostic != "" {
				status.State = mcpclient.StateError
				status.Error = diagnostic
			} else {
				status.State = mcpclient.StateConnected
			}
			resolved <- resolution{index: index, status: status, connection: connection, entry: entry}
		}(index, name, config, status)
	}
	retryGeneration := false
	for range pending {
		result := <-resolved
		if errors.Is(result.err, errGenerationChanged) {
			retryGeneration = true
		}
		lease.statuses[result.index] = result.status
		if result.connection != nil {
			lease.entries = append(lease.entries, result.entry)
			lease.tools = append(lease.tools, result.connection.Tools()...)
		}
	}
	if retryGeneration && ctx.Err() == nil {
		lease.Close()
		return manager.acquire(ctx, workspace, globalOnly)
	}
	return lease
}
