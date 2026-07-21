package web

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/internal/app/providerconfig"
	"github.com/ktsoator/or/coding/internal/app/usage"
	"github.com/ktsoator/or/coding/internal/app/workspace"
	"github.com/ktsoator/or/llm"
)

// Run serves the workspace's multi-session HTTP API at cfg.Addr. The React
// front-end runs as a separate process or deployment and consumes this API.
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
	manager, err := NewSessionManager(ctx, cfg, ledger, workspaces)
	if err != nil {
		return err
	}

	registry := llm.DefaultProviderRegistry()
	providers, err := providerconfig.NewStore(cfg.DataDir, registry)
	if err != nil {
		return err
	}
	providers.Apply()

	server := &Server{
		ctx:            ctx,
		sessions:       manager,
		ledger:         ledger,
		workspaces:     workspaces,
		registry:       registry,
		providers:      providers,
		browseRoot:     cfg.Cwd,
		frontendOrigin: cfg.FrontendOrigin,
	}
	fmt.Println("coding API")
	fmt.Printf("listening on http://%s/api/\n", cfg.Addr)
	if cfg.FrontendOrigin != "" {
		fmt.Printf("allowing front-end origin %s\n", cfg.FrontendOrigin)
	}
	return http.ListenAndServe(cfg.Addr, server.Handler())
}
