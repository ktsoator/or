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
	"strings"
	"syscall"
	"time"

	"github.com/ktsoator/or/coding/internal/app"
	"github.com/ktsoator/or/coding/internal/desktopserver"

	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
)

const (
	tokenEnvironment   = "CODING_DESKTOP_TOKEN"
	dataDirEnvironment = "OR_DATA_DIR"
)

type readyMessage struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	PreviewURL string `json:"previewURL"`
	CookieName string `json:"cookieName"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "coding desktop sidecar: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dataDir := os.Getenv(dataDirEnvironment)
	flags := flag.NewFlagSet("coding-desktop", flag.ContinueOnError)
	assets := flags.String("assets", "", "directory containing the built web client")
	flags.StringVar(&dataDir, "data-dir", dataDir, "coding data directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dataDir, err := resolveDataDir(dataDir)
	if err != nil {
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
	productRuntime, err := app.New(ctx, dataDir)
	if err != nil {
		return err
	}
	defer productRuntime.Close()

	appListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer appListener.Close()
	previewListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer previewListener.Close()

	handler := desktopserver.New(productRuntime.Handler(), os.DirFS(assetDir), token)
	appServer := &http.Server{Handler: handler}
	previewServer := &http.Server{Handler: productRuntime.PreviewHandler()}
	serverErrors := make(chan error, 2)
	go func() { serverErrors <- appServer.Serve(appListener) }()
	go func() { serverErrors <- previewServer.Serve(previewListener) }()

	ready := readyMessage{
		Type:       "ready",
		URL:        "http://" + appListener.Addr().String(),
		PreviewURL: "http://" + previewListener.Addr().String(),
		CookieName: desktopserver.CookieName,
	}
	if err := json.NewEncoder(os.Stdout).Encode(ready); err != nil {
		return fmt.Errorf("write ready message: %w", err)
	}

	stopped := 0
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serverErrors:
		stopped = 1
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = appServer.Shutdown(shutdownCtx)
	_ = previewServer.Shutdown(shutdownCtx)
	for stopped < 2 {
		err := <-serverErrors
		stopped++
		if serveErr == nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func resolveDataDir(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve data directory: %w", err)
		}
		value = filepath.Join(home, ".or", "coding")
	}
	return filepath.Abs(value)
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
