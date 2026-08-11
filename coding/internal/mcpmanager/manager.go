// Package mcpmanager owns application-lifetime MCP connections and leases
// immutable tool snapshots to conversation runtimes.
package mcpmanager

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ktsoator/or/coding/internal/mcpclient"
)

const defaultFailureRetryDelay = 15 * time.Second

var errGenerationChanged = errors.New("MCP configuration changed while acquiring connections")

type managedConnection interface {
	Transport() string
	Tools() []mcpclient.Tool
	Diagnostic() string
	Close()
}

type connectFunc func(context.Context, string, mcpclient.ServerConfig, string) (managedConnection, error)

type connectionKey struct {
	generation uint64
	server     string
	scope      string
}

type connectionEntry struct {
	key       connectionKey
	ready     chan struct{}
	config    mcpclient.ServerConfig
	workspace string

	connection managedConnection
	err        error
	failedAt   time.Time
	complete   bool
	stale      bool
	references int
}

type configStamp struct {
	exists bool
	digest [sha256.Size]byte
}

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

func connectServer(ctx context.Context, name string, config mcpclient.ServerConfig, workspace string) (managedConnection, error) {
	return mcpclient.Connect(ctx, name, config, workspace)
}

// Path returns the private configuration file managed by this instance.
func (manager *Manager) Path() string { return manager.path }

// Reload installs the latest on-disk configuration as a new generation. Old
// connections remain alive until every lease using them has been released.
func (manager *Manager) Reload() error {
	manager.reloadMu.Lock()
	defer manager.reloadMu.Unlock()
	return manager.reload()
}

func (manager *Manager) reload() error {
	config, stamp, err := loadConfig(manager.path)

	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return context.Canceled
	}
	manager.generation++
	manager.config = config
	manager.configErr = err
	manager.configStamp = stamp
	var closing []managedConnection
	for key, entry := range manager.entries {
		entry.stale = true
		if entry.complete && entry.references == 0 {
			delete(manager.entries, key)
			if entry.connection != nil {
				closing = append(closing, entry.connection)
			}
		}
	}
	manager.mu.Unlock()
	for _, connection := range closing {
		connection.Close()
	}
	return err
}

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

func (manager *Manager) reloadIfChanged() error {
	stamp := readConfigStamp(manager.path)
	manager.mu.Lock()
	unchanged := stamp == manager.configStamp
	closed := manager.closed
	manager.mu.Unlock()
	if unchanged || closed {
		return nil
	}
	manager.reloadMu.Lock()
	defer manager.reloadMu.Unlock()
	stamp = readConfigStamp(manager.path)
	manager.mu.Lock()
	unchanged = stamp == manager.configStamp
	manager.mu.Unlock()
	if unchanged {
		return nil
	}
	return manager.reload()
}

func (manager *Manager) snapshotConfig() (mcpclient.Config, uint64, bool, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return cloneConfig(manager.config), manager.generation, manager.closed, manager.configErr
}

func (manager *Manager) acquireConnection(
	ctx context.Context,
	key connectionKey,
	config mcpclient.ServerConfig,
	workspace string,
) (managedConnection, *connectionEntry, error) {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil, nil, context.Canceled
	}
	if key.generation != manager.generation {
		manager.mu.Unlock()
		return nil, nil, errGenerationChanged
	}
	entry := manager.entries[key]
	if entry != nil && entry.complete && entry.err != nil && entry.references == 0 &&
		time.Since(entry.failedAt) >= manager.retryDelay {
		delete(manager.entries, key)
		entry = nil
	}
	if entry == nil {
		entry = &connectionEntry{
			key:       key,
			ready:     make(chan struct{}),
			config:    config,
			workspace: workspace,
		}
		manager.entries[key] = entry
		manager.background.Add(1)
		go manager.connectEntry(entry)
	}
	entry.references++
	manager.mu.Unlock()

	select {
	case <-entry.ready:
		if entry.err != nil {
			manager.releaseEntry(entry)
			return nil, nil, entry.err
		}
		return entry.connection, entry, nil
	case <-ctx.Done():
		manager.releaseEntry(entry)
		return nil, nil, ctx.Err()
	}
}

func (manager *Manager) connectEntry(entry *connectionEntry) {
	defer manager.background.Done()
	connection, err := manager.connect(manager.ctx, entry.key.server, entry.config, entry.workspace)

	manager.mu.Lock()
	entry.connection = connection
	entry.err = err
	entry.complete = true
	if err != nil {
		entry.failedAt = time.Now()
	}
	close(entry.ready)
	shouldClose := (manager.closed || entry.stale) && entry.references == 0 && connection != nil
	if shouldClose && manager.entries[entry.key] == entry {
		delete(manager.entries, entry.key)
	}
	manager.mu.Unlock()
	if shouldClose {
		connection.Close()
	}
}

func (manager *Manager) releaseEntry(entry *connectionEntry) {
	if entry == nil {
		return
	}
	manager.mu.Lock()
	if entry.references > 0 {
		entry.references--
	}
	shouldClose := entry.complete && entry.references == 0 && (manager.closed || entry.stale) && entry.connection != nil
	if shouldClose && manager.entries[entry.key] == entry {
		delete(manager.entries, entry.key)
	}
	manager.mu.Unlock()
	if shouldClose {
		entry.connection.Close()
	}
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

func loadConfig(path string) (mcpclient.Config, configStamp, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return mcpclient.Config{Version: 1, MCPServers: make(map[string]mcpclient.ServerConfig)}, configStamp{}, nil
	}
	if err != nil {
		return mcpclient.Config{}, configStamp{}, err
	}
	stamp := configStamp{exists: true, digest: sha256.Sum256(data)}
	config, err := mcpclient.ParseConfig(data)
	return config, stamp, err
}

func readConfigStamp(path string) configStamp {
	data, err := os.ReadFile(path)
	if err != nil {
		return configStamp{}
	}
	return configStamp{exists: true, digest: sha256.Sum256(data)}
}

func cloneConfig(config mcpclient.Config) mcpclient.Config {
	cloned := mcpclient.Config{Version: config.Version, MCPServers: make(map[string]mcpclient.ServerConfig, len(config.MCPServers))}
	for name, server := range config.MCPServers {
		server.Args = append([]string(nil), server.Args...)
		server.Workspaces = append([]string(nil), server.Workspaces...)
		server.Env = cloneMap(server.Env)
		server.Headers = cloneMap(server.Headers)
		cloned.MCPServers[name] = server
	}
	return cloned
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func normalizeWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func connectionScope(config mcpclient.ServerConfig, workspace string) string {
	if strings.TrimSpace(config.Command) != "" || referencesWorkspace(config) {
		return workspace
	}
	return "global"
}

func referencesWorkspace(config mcpclient.ServerConfig) bool {
	values := []string{config.Command, config.Cwd, config.URL}
	values = append(values, config.Args...)
	for _, value := range config.Env {
		values = append(values, value)
	}
	for _, value := range config.Headers {
		values = append(values, value)
	}
	for _, value := range values {
		if strings.Contains(value, "${workspace}") {
			return true
		}
	}
	return false
}

func transportName(config mcpclient.ServerConfig) string {
	if strings.TrimSpace(config.Command) != "" {
		return "stdio"
	}
	if strings.TrimSpace(config.URL) != "" {
		return "streamable_http"
	}
	return ""
}
