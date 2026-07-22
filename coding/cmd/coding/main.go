// Command coding starts the product API consumed by the separate client.
package main

import (
	"context"
	"fmt"
	"os"

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

	if err := app.Run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}
}
