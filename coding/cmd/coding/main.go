// Command coding serves the multi-session HTTP API consumed by the separate
// React front-end.
package main

import (
	"context"
	"fmt"
	"os"

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

	if err := web.Run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}
}
