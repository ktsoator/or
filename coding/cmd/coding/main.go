// Command coding is a minimal coding agent: an interactive print-mode loop that
// reads requests, runs tool-using turns against a model, and persists the
// session. It is the product shell over the embeddable coding core.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/internal/app/mode"

	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
)

func main() {
	cfg := config.Defaults()

	flag.StringVar(&cfg.Provider, "provider", cfg.Provider, "model provider (e.g. anthropic, openai)")
	flag.StringVar(&cfg.Model, "model", cfg.Model, "model id")
	flag.StringVar(&cfg.Cwd, "cwd", cfg.Cwd, "workspace root directory")
	flag.StringVar(&cfg.SessionFile, "session", cfg.SessionFile, "session transcript file (default .coding/session.jsonl under cwd)")
	flag.StringVar(&cfg.ThinkingLevel, "thinking", cfg.ThinkingLevel, "reasoning level: off, minimal, low, medium, high")
	flag.Parse()

	if err := cfg.Resolve(); err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}

	if err := mode.RunPrint(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "coding: %v\n", err)
		os.Exit(1)
	}
}
