package mcp

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
)

type configStamp struct {
	exists bool
	digest [sha256.Size]byte
}

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

func (manager *Manager) snapshotConfig() (Config, uint64, bool, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return cloneConfig(manager.config), manager.generation, manager.closed, manager.configErr
}

func loadConfig(path string) (Config, configStamp, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Version: 1, MCPServers: make(map[string]ServerConfig)}, configStamp{}, nil
	}
	if err != nil {
		return Config{}, configStamp{}, err
	}
	stamp := configStamp{exists: true, digest: sha256.Sum256(data)}
	config, err := ParseConfig(data)
	return config, stamp, err
}

func readConfigStamp(path string) configStamp {
	data, err := os.ReadFile(path)
	if err != nil {
		return configStamp{}
	}
	return configStamp{exists: true, digest: sha256.Sum256(data)}
}

func cloneConfig(config Config) Config {
	cloned := Config{Version: config.Version, MCPServers: make(map[string]ServerConfig, len(config.MCPServers))}
	for name, server := range config.MCPServers {
		server.Args = append([]string(nil), server.Args...)
		server.Workspaces = append([]string(nil), server.Workspaces...)
		server.Env = cloneMap(server.Env)
		server.Headers = cloneMap(server.Headers)
		cloned.MCPServers[name] = server
	}
	return cloned
}
