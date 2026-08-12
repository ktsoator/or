package mcpmanager

import (
	"sync"

	"github.com/ktsoator/or/coding/internal/mcpclient"
)

// Lease is one conversation's immutable view of tools and server statuses.
type Lease struct {
	manager   *Manager
	entries   []*connectionEntry
	tools     []mcpclient.Tool
	statuses  []mcpclient.ServerStatus
	closeOnce sync.Once
}

// Tools returns an independent slice of protocol-native tools.
func (lease *Lease) Tools() []mcpclient.Tool {
	if lease == nil {
		return nil
	}
	return append([]mcpclient.Tool(nil), lease.tools...)
}

// Statuses returns an independent slice of secret-free diagnostics.
func (lease *Lease) Statuses() []mcpclient.ServerStatus {
	if lease == nil {
		return nil
	}
	return append([]mcpclient.ServerStatus(nil), lease.statuses...)
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
