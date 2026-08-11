package mcpmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/mcpclient"
)

type fakeConnection struct {
	transport string
	closed    atomic.Int32
}

func (connection *fakeConnection) Transport() string { return connection.transport }
func (*fakeConnection) Tools() []mcpclient.Tool      { return nil }
func (*fakeConnection) Diagnostic() string           { return "" }
func (connection *fakeConnection) Close()            { connection.closed.Add(1) }

func TestAcquireConnectsServersConcurrentlyAndReusesConnections(t *testing.T) {
	path := writeTestConfig(t, map[string]mcpclient.ServerConfig{
		"alpha": {URL: "https://alpha.example/mcp"},
		"beta":  {URL: "https://beta.example/mcp"},
	})
	manager := New(t.Context(), path)
	t.Cleanup(manager.Close)
	started := make(chan string, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	manager.connect = func(_ context.Context, name string, _ mcpclient.ServerConfig, _ string) (managedConnection, error) {
		calls.Add(1)
		started <- name
		<-release
		return &fakeConnection{transport: "streamable_http"}, nil
	}

	leaseReady := make(chan *Lease, 1)
	go func() { leaseReady <- manager.Acquire(t.Context(), t.TempDir()) }()
	seen := map[string]bool{}
	for range 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(time.Second):
			t.Fatal("servers did not start concurrently")
		}
	}
	close(release)
	lease := <-leaseReady
	if !seen["alpha"] || !seen["beta"] {
		t.Fatalf("started = %v", seen)
	}
	assertConnectedStatuses(t, lease.Statuses(), 2)
	lease.Close()

	reused := manager.Acquire(t.Context(), t.TempDir())
	assertConnectedStatuses(t, reused.Statuses(), 2)
	reused.Close()
	if calls.Load() != 2 {
		t.Fatalf("connect calls = %d, want 2", calls.Load())
	}
}

func TestAcquireSharesGlobalHTTPAndIsolatesWorkspaceConnections(t *testing.T) {
	path := writeTestConfig(t, map[string]mcpclient.ServerConfig{
		"global":    {URL: "https://global.example/mcp"},
		"templated": {URL: "https://workspace.example/mcp?root=${workspace}"},
		"stdio":     {Command: "example"},
	})
	manager := New(t.Context(), path)
	t.Cleanup(manager.Close)
	var mu sync.Mutex
	calls := make(map[string]int)
	manager.connect = func(_ context.Context, name string, _ mcpclient.ServerConfig, _ string) (managedConnection, error) {
		mu.Lock()
		calls[name]++
		mu.Unlock()
		return &fakeConnection{}, nil
	}

	first := manager.Acquire(t.Context(), t.TempDir())
	first.Close()
	second := manager.Acquire(t.Context(), t.TempDir())
	second.Close()
	mu.Lock()
	defer mu.Unlock()
	if calls["global"] != 1 || calls["templated"] != 2 || calls["stdio"] != 2 {
		t.Fatalf("connect calls = %v", calls)
	}
}

func TestReloadKeepsLeasedGenerationAlive(t *testing.T) {
	path := writeTestConfig(t, map[string]mcpclient.ServerConfig{
		"demo": {URL: "https://old.example/mcp"},
	})
	manager := New(t.Context(), path)
	t.Cleanup(manager.Close)
	var mu sync.Mutex
	var connections []*fakeConnection
	manager.connect = func(_ context.Context, _ string, _ mcpclient.ServerConfig, _ string) (managedConnection, error) {
		connection := &fakeConnection{transport: "streamable_http"}
		mu.Lock()
		connections = append(connections, connection)
		mu.Unlock()
		return connection, nil
	}

	oldLease := manager.Acquire(t.Context(), t.TempDir())
	if err := mcpclient.WriteConfig(path, mcpclient.Config{MCPServers: map[string]mcpclient.ServerConfig{
		"demo": {URL: "https://new.example/mcp"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	oldConnection := connections[0]
	mu.Unlock()
	if oldConnection.closed.Load() != 0 {
		t.Fatal("reload closed a connection with an active lease")
	}

	newLease := manager.Acquire(t.Context(), t.TempDir())
	mu.Lock()
	connectionCount := len(connections)
	mu.Unlock()
	if connectionCount != 2 {
		t.Fatalf("connections = %d, want 2 generations", connectionCount)
	}
	oldLease.Close()
	if oldConnection.closed.Load() != 1 {
		t.Fatalf("old connection close count = %d, want 1", oldConnection.closed.Load())
	}
	newLease.Close()
}

func TestAcquireCachesFailuresUntilRetryWindow(t *testing.T) {
	path := writeTestConfig(t, map[string]mcpclient.ServerConfig{
		"broken": {URL: "https://broken.example/mcp"},
	})
	manager := New(t.Context(), path)
	t.Cleanup(manager.Close)
	manager.retryDelay = time.Hour
	var calls atomic.Int32
	manager.connect = func(context.Context, string, mcpclient.ServerConfig, string) (managedConnection, error) {
		calls.Add(1)
		return nil, errors.New("unavailable")
	}

	for range 2 {
		lease := manager.Acquire(t.Context(), t.TempDir())
		statuses := lease.Statuses()
		lease.Close()
		if len(statuses) != 1 || statuses[0].State != mcpclient.StateError || statuses[0].Error != "unavailable" {
			t.Fatalf("statuses = %#v", statuses)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("connect calls = %d, want 1 cached failure", calls.Load())
	}
}

func TestManagerCloseDoesNotDoubleCloseLeasedConnection(t *testing.T) {
	path := writeTestConfig(t, map[string]mcpclient.ServerConfig{
		"demo": {URL: "https://demo.example/mcp"},
	})
	manager := New(t.Context(), path)
	connection := &fakeConnection{}
	manager.connect = func(context.Context, string, mcpclient.ServerConfig, string) (managedConnection, error) {
		return connection, nil
	}
	lease := manager.Acquire(t.Context(), t.TempDir())
	manager.Close()
	lease.Close()
	if connection.closed.Load() != 1 {
		t.Fatalf("connection close count = %d, want 1", connection.closed.Load())
	}
}

func TestAcquireSurfacesConfigurationErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"broken":{"command":"x","unknown":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.Context(), path)
	t.Cleanup(manager.Close)
	lease := manager.Acquire(t.Context(), t.TempDir())
	defer lease.Close()
	statuses := lease.Statuses()
	if len(statuses) != 1 || statuses[0].Name != "configuration" || statuses[0].State != mcpclient.StateError {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func writeTestConfig(t *testing.T, servers map[string]mcpclient.ServerConfig) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := mcpclient.WriteConfig(path, mcpclient.Config{MCPServers: servers}); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertConnectedStatuses(t *testing.T, statuses []mcpclient.ServerStatus, count int) {
	t.Helper()
	if len(statuses) != count {
		t.Fatalf("statuses = %#v", statuses)
	}
	for _, status := range statuses {
		if status.State != mcpclient.StateConnected {
			t.Fatalf("status = %#v", status)
		}
	}
}
