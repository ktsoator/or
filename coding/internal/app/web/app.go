package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/internal/app/providerconfig"
	"github.com/ktsoator/or/llm"
)

// Run serves the workspace's multi-session HTTP API at cfg.Addr. The React
// front-end runs as a separate process or deployment and consumes this API.
func Run(ctx context.Context, cfg config.Config) error {
	manager, err := NewSessionManager(ctx, cfg)
	if err != nil {
		return err
	}

	registry := llm.DefaultProviderRegistry()
	providers, err := providerconfig.NewStore(
		cfg.DataDir,
		registry,
	)
	if err != nil {
		return err
	}
	providers.Apply()

	server := NewServer(ctx, manager, registry, providers, cfg.FrontendOrigin)
	fmt.Println("coding API")
	fmt.Printf("listening on http://%s/api/\n", cfg.Addr)
	if cfg.FrontendOrigin != "" {
		fmt.Printf("allowing front-end origin %s\n", cfg.FrontendOrigin)
	}
	return http.ListenAndServe(cfg.Addr, server.Handler())
}
