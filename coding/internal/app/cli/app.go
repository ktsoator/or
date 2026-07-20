// Package cli is the terminal product adapter for a coding session.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"

	"github.com/ktsoator/or/coding/internal/app/bootstrap"
	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/internal/app/providerconfig"
	"github.com/ktsoator/or/coding/policy"
	"github.com/ktsoator/or/llm"
)

// Run starts an interactive terminal session. It reads prompts from stdin until
// EOF or an "exit"/"quit" line, and returns when the loop ends.
func Run(ctx context.Context, cfg config.Config) error {
	reader := bufio.NewReader(os.Stdin)
	providers, err := providerconfig.NewStore(cfg.DataDir, llm.DefaultProviderRegistry())
	if err != nil {
		return err
	}
	providers.Apply()
	selection, ok := providers.ActiveModel()
	if !ok {
		return fmt.Errorf("no model is configured; start coding with -web and configure one in Settings")
	}
	cfg.Provider = selection.Provider
	cfg.Model = selection.Model
	cfg.ThinkingLevel = string(selection.ThinkingLevel)

	session, err := bootstrap.NewSession(ctx, cfg, bootstrap.Dependencies{
		Confirm: confirmer(reader, os.Stdout),
	})
	if err != nil {
		return err
	}
	defer session.Close()

	printer := NewRenderer(os.Stdout)
	unsubscribe := session.Subscribe(printer.Handle)
	defer unsubscribe()

	var interrupted atomic.Bool
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		for range sig {
			interrupted.Store(true)
			session.Abort()
		}
	}()

	model := session.Snapshot().Model
	fmt.Printf("coding agent — %s/%s in %s\n", model.Provider, model.ID, session.Cwd())
	fmt.Println("type a request; Ctrl-C interrupts a run, 'exit' or Ctrl-D quits.")

	for {
		fmt.Print("\n\033[1m›\033[0m ")
		line, err := reader.ReadString('\n')
		text := strings.TrimSpace(line)
		if text == "exit" || text == "quit" {
			return nil
		}
		if text != "" {
			interrupted.Store(false)
			if runErr := session.Prompt(ctx, text); runErr != nil {
				if interrupted.Load() {
					fmt.Fprintln(os.Stdout, "\n^C interrupted")
				} else {
					fmt.Fprintf(os.Stderr, "\nerror: %v\n", runErr)
				}
			}
		}
		if err != nil {
			return nil
		}
	}
}

func confirmer(reader *bufio.Reader, out io.Writer) policy.Confirm {
	return func(req policy.Request) bool {
		fmt.Fprintf(out, "\n\033[33mallow %s?\033[0m [y/N] ", describe(req))
		answer, _ := reader.ReadString('\n')
		return strings.EqualFold(strings.TrimSpace(answer), "y")
	}
}

func describe(req policy.Request) string {
	switch req.Tool {
	case "bash":
		if cmd, ok := req.Args["command"].(string); ok {
			return "bash: " + firstLine(cmd)
		}
	case "edit", "write":
		if path, ok := req.Args["path"].(string); ok {
			return req.Tool + " " + path
		}
	}
	return req.Tool
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before + " …"
	}
	return s
}
