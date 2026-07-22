// Command coding starts the product API consumed by the separate client.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ktsoator/or/coding/internal/app"
	"github.com/ktsoator/or/coding/internal/config"

	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if config.IsHelp(err) {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}
}
