// Command coding-desktop runs the authenticated loopback server supervised by
// the Electron main process.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ktsoator/or/coding/internal/app"
	"github.com/ktsoator/or/coding/internal/config"
	"github.com/ktsoator/or/coding/internal/desktopserver"

	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
)

const tokenEnvironment = "CODING_DESKTOP_TOKEN"

type readyMessage struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	CookieName string `json:"cookieName"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "coding desktop sidecar: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg := config.Defaults()
	flags := flag.NewFlagSet("coding-desktop", flag.ContinueOnError)
	assets := flags.String("assets", "", "directory containing the built web client")
	flags.StringVar(&cfg.Cwd, "cwd", cfg.Cwd, "initial directory-browser location")
	flags.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "coding data directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := cfg.Resolve(); err != nil {
		return err
	}
	assetDir, err := validateAssets(*assets)
	if err != nil {
		return err
	}
	token := os.Getenv(tokenEnvironment)
	if len(token) < 32 {
		return fmt.Errorf("%s must contain at least 32 characters", tokenEnvironment)
	}

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalContext)
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		cancel()
	}()
	productRuntime, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer productRuntime.Close()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()

	handler := desktopserver.New(productRuntime.Handler(), os.DirFS(assetDir), token)
	server := &http.Server{Handler: handler}
	shutdownDone := make(chan struct{})
	defer close(shutdownDone)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-shutdownDone:
		}
	}()

	ready := readyMessage{
		Type:       "ready",
		URL:        "http://" + listener.Addr().String(),
		CookieName: desktopserver.CookieName,
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		return fmt.Errorf("write ready message: %w", err)
	}

	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
		return nil
	}
	return err
}

func validateAssets(value string) (string, error) {
	if value == "" {
		return "", errors.New("-assets is required")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(filepath.Join(abs, "index.html"))
	if err != nil {
		return "", fmt.Errorf("validate client assets: %w", err)
	}
	if info.IsDir() {
		return "", errors.New("client index.html is a directory")
	}
	return abs, nil
}
