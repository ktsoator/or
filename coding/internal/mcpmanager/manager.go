// Package mcpmanager owns application-lifetime MCP connections and leases
// immutable tool snapshots to conversation runtimes.
package mcpmanager

import (
	"context"
	"sync"
	"time"

	"github.com/ktsoator/or/coding/internal/mcpclient"
)

const defaultFailureRetryDelay = 15 * time.Second

// Manager loads one product-owned config and shares matching connections among
// conversation runtimes. Config generations are isolated so active runtimes
// retain their old connections while newly loaded runtimes use new settings.
type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc
	path   string

	mu          sync.Mutex
	reloadMu    sync.Mutex
	config      mcpclient.Config
	configErr   error
	configStamp configStamp
	generation  uint64
	entries     map[connectionKey]*connectionEntry
	closed      bool
	connect     connectFunc
	retryDelay  time.Duration
	background  sync.WaitGroup
	closeOnce   sync.Once
}

// New creates a manager and loads its initial configuration. Configuration
// errors are retained as lease diagnostics instead of preventing app startup.
func New(ctx context.Context, path string) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	manager := &Manager{
		ctx:        ctx,
		cancel:     cancel,
		path:       path,
		entries:    make(map[connectionKey]*connectionEntry),
		connect:    connectServer,
		retryDelay: defaultFailureRetryDelay,
	}
	_ = manager.Reload()
	return manager
}

// Path returns the private configuration file managed by this instance.
func (manager *Manager) Path() string { return manager.path }

// Warm starts resolving the servers visible to workspace without retaining a
// conversation lease. Connections from the active config generation remain
// cached for later sessions.
func (manager *Manager) Warm(workspace string) {
	manager.warm(workspace, false)
}

// WarmGlobal starts workspace-independent HTTP servers. It is safe during app
// startup before any project or scratch workspace has been selected.
func (manager *Manager) WarmGlobal() {
	manager.warm("", true)
}

func (manager *Manager) warm(workspace string, globalOnly bool) {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return
	}
	manager.background.Add(1)
	manager.mu.Unlock()
	go func() {
		defer manager.background.Done()
		lease := manager.acquire(manager.ctx, workspace, globalOnly)
		lease.Close()
	}()
}

// Close cancels initialization and closes every cached connection.
func (manager *Manager) Close() {
	manager.closeOnce.Do(func() {
		manager.mu.Lock()
		manager.closed = true
		manager.mu.Unlock()
		manager.cancel()
		manager.background.Wait()

		manager.mu.Lock()
		connections := make([]managedConnection, 0, len(manager.entries))
		for _, entry := range manager.entries {
			if entry.connection != nil {
				connections = append(connections, entry.connection)
				entry.connection = nil
			}
		}
		manager.entries = make(map[connectionKey]*connectionEntry)
		manager.mu.Unlock()
		for _, connection := range connections {
			connection.Close()
		}
	})
}
