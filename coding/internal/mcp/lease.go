package mcp

import (
	"sync"

	"github.com/ktsoator/or/coding/internal/mcp/client"
	"github.com/ktsoator/or/coding/internal/tools"
)

// Lease is one conversation's immutable view of tools and server statuses.
type Lease struct {
	manager       *Manager
	entries       []*connectionEntry
	protocolTools []client.Tool
	statuses      []ServerStatus
	toolsOnce     sync.Once
	tools         []tools.Tool
	closeOnce     sync.Once
}

// Tools returns an independent slice of MCP tools adapted to Or's tool and
// permission contracts.
func (lease *Lease) Tools() []tools.Tool {
	if lease == nil {
		return nil
	}
	lease.toolsOnce.Do(func() {
		lease.tools = buildTools(lease.protocolTools, lease.statuses)
	})
	return append([]tools.Tool(nil), lease.tools...)
}

// Statuses returns an independent slice of secret-free diagnostics.
func (lease *Lease) Statuses() []ServerStatus {
	if lease == nil {
		return nil
	}
	return append([]ServerStatus(nil), lease.statuses...)
}

// Close releases this lease without closing connections still used by another
// runtime or retained by the active manager generation.
func (lease *Lease) Close() {
	if lease == nil || lease.manager == nil {
		return
	}
	lease.closeOnce.Do(func() {
		for _, entry := range lease.entries {
			lease.manager.releaseEntry(entry)
		}
	})
}
