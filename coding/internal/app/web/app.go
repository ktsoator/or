package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ktsoator/or/coding/internal/app/config"
)

// Run serves the workspace's multi-session HTTP API at cfg.Addr. The React
// front-end runs as a separate process or deployment and consumes this API.
func Run(ctx context.Context, cfg config.Config) error {
	manager, err := NewSessionManager(ctx, cfg)
	if err != nil {
		return err
	}

	server := NewServer(ctx, manager, cfg.FrontendOrigin)
	model, _ := cfg.ResolveModel()
	fmt.Printf("coding API — %s/%s\n", model.Provider, model.ID)
	fmt.Printf("listening on http://%s/api/\n", cfg.Addr)
	if cfg.FrontendOrigin != "" {
		fmt.Printf("allowing front-end origin %s\n", cfg.FrontendOrigin)
	}
	return http.ListenAndServe(cfg.Addr, server.Handler())
}
