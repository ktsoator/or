// Command coding-desktop runs the Coding product in a native Wails window.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ktsoator/or/coding/internal/app"
	"github.com/ktsoator/or/coding/internal/config"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	_ "github.com/ktsoator/or/llm/all" // register the built-in protocol adapters
)

// Wails copies coding/client/dist here for production builds, then restores the
// directory to its tracked placeholder so generated assets stay out of Git.
//
//go:embed all:frontend/dist
var assets embed.FS

const singleInstanceID = "com.ktsoator.or.coding"

type DesktopBridge struct {
	mu               sync.RWMutex
	ctx              context.Context
	defaultDirectory string
}

func (d *DesktopBridge) set(ctx context.Context) {
	d.mu.Lock()
	d.ctx = ctx
	d.mu.Unlock()
}

func (d *DesktopBridge) context() context.Context {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ctx
}

// ChooseDirectory opens the platform folder picker. An empty result means the
// user cancelled; workspace validation still happens through the HTTP API.
func (d *DesktopBridge) ChooseDirectory(initialPath, title string) (string, error) {
	ctx := d.context()
	if ctx == nil {
		return "", errors.New("desktop window is not ready")
	}
	if strings.TrimSpace(title) == "" {
		title = "Choose a workspace folder"
	}
	return wailsRuntime.OpenDirectoryDialog(ctx, wailsRuntime.OpenDialogOptions{
		Title:                title,
		DefaultDirectory:     firstExistingDirectory(initialPath, d.defaultDirectory),
		CanCreateDirectories: true,
		ResolvesAliases:      true,
	})
}

func firstExistingDirectory(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func (d *DesktopBridge) focus() {
	ctx := d.context()
	if ctx == nil {
		return
	}
	time.AfterFunc(100*time.Millisecond, func() {
		wailsRuntime.WindowUnminimise(ctx)
		wailsRuntime.Show(ctx)
		wailsRuntime.WindowShow(ctx)
	})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "coding desktop: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Defaults()
	if err := cfg.Resolve(); err != nil {
		return err
	}

	productRuntime, err := app.New(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer productRuntime.Close()

	desktop := &DesktopBridge{defaultDirectory: cfg.Cwd}

	return wails.Run(&options.App{
		Title:     "Coding",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: productRuntime.Handler(),
		},
		BackgroundColour: options.NewRGB(255, 255, 255),
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: singleInstanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				desktop.focus()
			},
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
		OnStartup: desktop.set,
		OnShutdown: func(context.Context) {
			productRuntime.Close()
		},
		Bind: []interface{}{desktop},
	})
}
