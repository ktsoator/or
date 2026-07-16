// Package mode holds the coding CLI's run modes. Print mode is a simple
// read-eval-print loop: read a line, run a turn, render the events, repeat.
package mode

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"

	"github.com/ktsoator/or/coding"
	"github.com/ktsoator/or/coding/internal/app/config"
	"github.com/ktsoator/or/coding/internal/app/render"
	"github.com/ktsoator/or/coding/policy"
	"github.com/ktsoator/or/coding/store"
)

// RunPrint starts an interactive print-mode session using cfg. It reads prompts
// from stdin until EOF or an "exit"/"quit" line, and returns when the loop ends.
func RunPrint(ctx context.Context, cfg config.Config) error {
	model, err := cfg.ResolveModel()
	if err != nil {
		return err
	}

	// One reader serves both the prompt loop and the permission confirmations,
	// so buffered input is never split between two readers on os.Stdin.
	reader := bufio.NewReader(os.Stdin)

	session, err := coding.New(ctx, coding.Options{
		Model:         model,
		ThinkingLevel: cfg.Thinking(),
		Cwd:           cfg.Cwd,
		Store:         store.NewJSONL(cfg.SessionFile),
		Policy:        policy.Gate{Confirm: confirmer(reader, os.Stdout)},
	})
	if err != nil {
		return err
	}

	printer := render.New(os.Stdout)
	unsubscribe := session.Subscribe(printer.Handle)
	defer unsubscribe()

	// Ctrl-C aborts the current run and returns to the prompt, rather than
	// killing the process. Each SIGINT flags the interrupt and asks the session
	// to abort; an idle Ctrl-C is a harmless no-op. Quit with 'exit' or Ctrl-D.
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
			return nil // EOF
		}
	}
}

// confirmer returns a permission Confirm that prompts on out and reads a y/N
// answer from reader.
func confirmer(reader *bufio.Reader, out io.Writer) policy.Confirm {
	return func(req policy.Request) bool {
		fmt.Fprintf(out, "\n\033[33mallow %s?\033[0m [y/N] ", describe(req))
		answer, _ := reader.ReadString('\n')
		return strings.EqualFold(strings.TrimSpace(answer), "y")
	}
}

// describe renders a short human-readable summary of a tool call for the
// confirmation prompt.
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

// firstLine returns the first line of s, for compact command display.
func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before + " …"
	}
	return s
}
