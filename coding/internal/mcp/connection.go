package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/ktsoator/or/coding/internal/mcp/client"
)

var errGenerationChanged = errors.New("MCP configuration changed while acquiring connections")

type managedConnection interface {
	Transport() string
	Tools() []client.Tool
	Diagnostic() string
	Close()
}

type connectFunc func(context.Context, string, ServerConfig, string) (managedConnection, error)

type connectionKey struct {
	generation uint64
	server     string
	scope      string
}

type connectionEntry struct {
	key       connectionKey
	ready     chan struct{}
	config    ServerConfig
	workspace string

	connection managedConnection
	err        error
	failedAt   time.Time
	complete   bool
	stale      bool
	references int
}

func connectServer(ctx context.Context, name string, config ServerConfig, workspace string) (managedConnection, error) {
	return client.Connect(ctx, name, config.connectionConfig(), workspace)
}

func (manager *Manager) acquireConnection(
	ctx context.Context,
	key connectionKey,
	config ServerConfig,
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
