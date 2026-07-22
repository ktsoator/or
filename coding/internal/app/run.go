package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ktsoator/or/coding/internal/config"
	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/httpapi"
	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Run starts the coding product and serves its HTTP API at cfg.Addr. The client
// runs as a separate process or deployment and consumes this API.
//
// This is the composition root: every store is built once here and handed to
// whoever needs it, so no component has to reach through another to find one.
func Run(ctx context.Context, cfg config.Config) error {
	sessionDir := filepath.Join(cfg.DataDir, "sessions")
	ledger, err := usage.NewStore(filepath.Join(cfg.DataDir, "usage", "events.jsonl"))
	if err != nil {
		return err
	}
	workspaces, err := workspace.NewRegistry(filepath.Join(sessionDir, "workspaces.json"))
	if err != nil {
		return err
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
		return err
	}

	registry := llm.DefaultProviderRegistry()
	providers, err := provider.NewStore(cfg.DataDir, registry)
	if err != nil {
		return err
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
	// Startup notes go to stderr, where this command's errors already go, so a
	// caller redirecting stdout is not handed a banner it did not ask for.
	fmt.Fprintf(os.Stderr, "coding API listening on http://%s/api/\n", cfg.Addr)
	fmt.Fprintf(os.Stderr, "sessions and transcripts in %s\n", cfg.DataDir)
	if cfg.ClientOrigin != "" {
		fmt.Fprintf(os.Stderr, "allowing client origin %s\n", cfg.ClientOrigin)
	}
	return http.ListenAndServe(cfg.Addr, server.Handler())
}
