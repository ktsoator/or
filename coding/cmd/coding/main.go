// Command coding is a minimal coding agent. By default it runs an interactive
// print-mode loop in the terminal; with -web it serves the multi-session HTTP
// API consumed by the separate React front-end.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ktsoator/or/coding/internal/app/cli"
	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/internal/app/web"

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

	ctx := context.Background()
	if cfg.Mode == config.ModeWeb {
		err = web.Run(ctx, cfg)
	} else {
		err = cli.Run(ctx, cfg)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}
}
