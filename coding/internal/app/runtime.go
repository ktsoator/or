package app

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/httpapi"
	"github.com/ktsoator/or/coding/internal/mcp"
	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Runtime owns the product services exposed by the desktop sidecar.
type Runtime struct {
	handler       http.Handler
	conversations *conversation.Manager
	mcp           *mcp.Manager
	ledger        *usage.Store
	cancel        context.CancelFunc
	closeOnce     sync.Once
}

// New assembles the product runtime served by the authenticated Electron
// sidecar. dataDir is already resolved by the process host.
func New(ctx context.Context, dataDir string) (*Runtime, error) {
	ctx, cancel := context.WithCancel(ctx)
	sessionDir := filepath.Join(dataDir, "sessions")
	ledger, err := usage.NewStore(filepath.Join(dataDir, "usage", "events.sqlite"))
	if err != nil {
		cancel()
		return nil, err
	}
	workspaces, err := workspace.NewRegistry(filepath.Join(sessionDir, "workspaces.json"))
	if err != nil {
		_ = ledger.Close()
		cancel()
		return nil, err
	}
	transports := httpapi.NewSessionTransports()
	registry := llm.DefaultProviderRegistry()
	providers, err := provider.NewStore(dataDir, registry)
	if err != nil {
		_ = ledger.Close()
		cancel()
		return nil, err
	}
	providers.Apply()
	providerTests := provider.NewConnectionTester(providers, llm.Complete)
	mcp := mcp.New(ctx, filepath.Join(dataDir, "mcp.json"))
	mcp.WarmGlobal()
	for _, registered := range workspaces.List() {
		mcp.Warm(registered.Path)
	}

	manager, err := conversation.NewManager(ctx, conversation.Options{
		DataDir:      dataDir,
		Usage:        ledger,
		Workspaces:   workspaces,
		NewTransport: transports.New,
		MCP:          mcp,
	})
	if err != nil {
		mcp.Close()
		_ = ledger.Close()
		cancel()
		return nil, err
	}

	server := httpapi.NewServer(httpapi.Options{
		Conversations: manager,
		Transports:    transports,
		Ledger:        ledger,
		Workspaces:    workspaces,
		Registry:      registry,
		Providers:     providers,
		ProviderTests: providerTests,
		MCP:           mcp,
	})
	return &Runtime{
		handler:       server.Handler(),
		conversations: manager,
		mcp:           mcp,
		ledger:        ledger,
		cancel:        cancel,
	}, nil
}

// Handler returns the complete desktop /api HTTP surface.
func (r *Runtime) Handler() http.Handler { return r.handler }

// Close cancels in-flight work and releases session-owned background processes.
func (r *Runtime) Close() {
	r.closeOnce.Do(func() {
		r.cancel()
		r.conversations.Close()
		r.mcp.Close()
		_ = r.ledger.Close()
	})
}
