package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ktsoator/or/coding/internal/config"
	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/httpapi"
	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Runtime owns the product services shared by the server and desktop shells.
type Runtime struct {
	handler       http.Handler
	conversations *conversation.Manager
	cancel        context.CancelFunc
	closeOnce     sync.Once
}

// New assembles one product runtime without choosing how its HTTP handler is
// hosted. The CLI uses a TCP server; Wails mounts it behind its asset server.
func New(ctx context.Context, cfg config.Config) (*Runtime, error) {
	ctx, cancel := context.WithCancel(ctx)
	sessionDir := filepath.Join(cfg.DataDir, "sessions")
	ledger, err := usage.NewStore(filepath.Join(cfg.DataDir, "usage", "events.jsonl"))
	if err != nil {
		cancel()
		return nil, err
	}
	workspaces, err := workspace.NewRegistry(filepath.Join(sessionDir, "workspaces.json"))
	if err != nil {
		cancel()
		return nil, err
	}
	manager, err := conversation.NewManager(ctx, conversation.Options{
		DataDir:    cfg.DataDir,
		Usage:      ledger,
		Workspaces: workspaces,
		NewTransport: func(string) conversation.Transport {
			return httpapi.NewSessionTransport()
		},
	})
	if err != nil {
		cancel()
		return nil, err
	}

	registry := llm.DefaultProviderRegistry()
	providers, err := provider.NewStore(cfg.DataDir, registry)
	if err != nil {
		manager.Close()
		cancel()
		return nil, err
	}
	providers.Apply()

	server := httpapi.NewServer(httpapi.Options{
		Context:       ctx,
		Conversations: manager,
		Ledger:        ledger,
		Workspaces:    workspaces,
		Registry:      registry,
		Providers:     providers,
		BrowseRoot:    cfg.Cwd,
		ClientOrigin:  cfg.ClientOrigin,
	})
	return &Runtime{
		handler:       server.Handler(),
		conversations: manager,
		cancel:        cancel,
	}, nil
}

// Handler returns the complete /api HTTP surface.
func (r *Runtime) Handler() http.Handler { return r.handler }

// Close cancels in-flight work and releases session-owned background processes.
func (r *Runtime) Close() {
	r.closeOnce.Do(func() {
		r.cancel()
		r.conversations.Close()
	})
}

// Run starts the standalone coding API at cfg.Addr.
func Run(ctx context.Context, cfg config.Config) error {
	runtime, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer runtime.Close()

	// Startup notes go to stderr, where this command's errors already go, so a
	// caller redirecting stdout is not handed a banner it did not ask for.
	fmt.Fprintf(os.Stderr, "coding API listening on http://%s/api/\n", cfg.Addr)
	fmt.Fprintf(os.Stderr, "sessions and transcripts in %s\n", cfg.DataDir)
	if cfg.ClientOrigin != "" {
		fmt.Fprintf(os.Stderr, "allowing client origin %s\n", cfg.ClientOrigin)
	}
	server := &http.Server{Addr: cfg.Addr, Handler: runtime.Handler()}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-stopped:
		}
	}()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
		return nil
	}
	return err
}
