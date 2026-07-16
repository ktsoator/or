package web

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/bootstrap"
	"github.com/ktsoator/or/coding/internal/app/config"
)

// Run serves a single-session browser front-end at cfg.Addr. It blocks until
// the HTTP server stops.
func Run(ctx context.Context, cfg config.Config) error {
	hub := NewHub()
	broker := NewConfirmBroker(hub)

	session, err := bootstrap.NewSession(ctx, cfg, bootstrap.Dependencies{
		Confirm: broker.Confirm,
	})
	if err != nil {
		return err
	}

	session.Subscribe(func(ev coding.Event) {
		if data, ok := ProjectEvent(ev); ok {
			hub.Broadcast(data)
		}
	})

	server := NewServer(ctx, session, hub, broker)
	model := session.Snapshot().Model
	fmt.Printf("coding agent — %s/%s in %s\n", model.Provider, model.ID, session.Cwd())
	fmt.Printf("open http://%s\n", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, server.Handler())
}
