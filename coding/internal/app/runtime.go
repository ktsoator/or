package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/ktsoator/or/coding/internal/conversation"
	"github.com/ktsoator/or/coding/internal/httpapi"
	"github.com/ktsoator/or/coding/internal/mcp"
	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/coding/internal/snapshot"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

// Runtime owns the product services exposed by the desktop sidecar.
type Runtime struct {
	handler        http.Handler
	previewHandler http.Handler
	conversations  *conversation.Manager
	mcp            *mcp.Manager
	ledger         *usage.Store
	recorder       observability.Recorder
	cancel         context.CancelFunc
	closeOnce      sync.Once
}

// New assembles the product runtime served by the authenticated Electron
// sidecar. dataDir is already resolved by the process host.
func New(ctx context.Context, dataDir string) (*Runtime, error) {
	ctx, cancel := context.WithCancel(ctx)
	sessionDir := filepath.Join(dataDir, "sessions")
	ledger, err := usage.NewStore(filepath.Join(dataDir, "usage", "events.sqlite"))
	if err != nil {
		cancel()
		return nil, err
	}
	workspaces, err := workspace.NewRegistry(filepath.Join(sessionDir, "workspaces.json"))
	if err != nil {
		_ = ledger.Close()
		cancel()
		return nil, err
	}
	transports := httpapi.NewSessionTransports()
	registry := llm.DefaultProviderRegistry()
	providers, err := provider.NewStore(dataDir, registry)
	if err != nil {
		_ = ledger.Close()
		cancel()
		return nil, err
	}
	providers.Apply()
	providerTests := provider.NewConnectionTester(providers, llm.Complete)
	mcp := mcp.New(ctx, filepath.Join(dataDir, "mcp.json"))
	mcp.WarmGlobal()
	for _, registered := range workspaces.List() {
		mcp.Warm(registered.Path)
	}
	observabilityLogPath := filepath.Join(dataDir, "logs", "observability.jsonl")
	recorder, recorderErr := observability.NewJSONL(
		observabilityLogPath,
		observability.FileOptions{},
	)
	var eventRecorder observability.Recorder = recorder
	var eventCleaner observability.SessionCleaner = recorder
	if recorderErr != nil {
		// Diagnostics are best-effort: an unavailable log path must not prevent
		// the coding runtime from starting.
		discard := observability.DiscardRecorder{}
		eventRecorder = discard
		eventCleaner = discard
	}
	legacySnapshots, snapshotErr := snapshot.OpenLegacyFileStore(
		filepath.Join(dataDir, "diagnostics", "requests"),
	)
	if snapshotErr != nil {
		// Do not create the retired directory on new installations. Cleanup is
		// best-effort when an older release did leave snapshot files behind.
		legacySnapshots = nil
	}

	manager, err := conversation.NewManager(ctx, conversation.Options{
		DataDir:      dataDir,
		Usage:        ledger,
		Workspaces:   workspaces,
		NewTransport: transports.New,
		MCP:          mcp,
		Recorder:     eventRecorder,
		SessionData: diagnosticSessionCleaner{
			observability: eventCleaner,
			requests:      snapshotCleaner(legacySnapshots),
		},
	})
	if err != nil {
		_ = eventRecorder.Close()
		mcp.Close()
		_ = ledger.Close()
		cancel()
		return nil, err
	}

	server := httpapi.NewServer(httpapi.Options{
		Conversations:        manager,
		Transports:           transports,
		Ledger:               ledger,
		Workspaces:           workspaces,
		Registry:             registry,
		Providers:            providers,
		ProviderTests:        providerTests,
		MCP:                  mcp,
		ObservabilityLogPath: observabilityLogPath,
		RequestSnapshots:     manager,
	})
	runtime := &Runtime{
		handler:        server.Handler(),
		previewHandler: transports.PreviewHandler(),
		conversations:  manager,
		mcp:            mcp,
		ledger:         ledger,
		recorder:       eventRecorder,
		cancel:         cancel,
	}
	eventRecorder.Record(observability.Event{Name: observability.ApplicationStarted})
	return runtime, nil
}

type diagnosticSessionCleaner struct {
	observability observability.SessionCleaner
	requests      snapshot.SessionCleaner
}

func (cleaner diagnosticSessionCleaner) DeleteSession(sessionID string) error {
	return errors.Join(
		cleaner.observability.DeleteSession(sessionID),
		cleaner.requests.DeleteSession(sessionID),
	)
}

func snapshotCleaner(store *snapshot.FileStore) snapshot.SessionCleaner {
	if store == nil {
		return snapshot.DiscardCleaner{}
	}
	return store
}

// Handler returns the complete desktop /api HTTP surface.
func (r *Runtime) Handler() http.Handler { return r.handler }

// PreviewHandler returns the grant-scoped, read-only workspace preview surface.
// The desktop host serves it on an origin that has no product authentication.
func (r *Runtime) PreviewHandler() http.Handler { return r.previewHandler }

// Close cancels in-flight work and releases session-owned background processes.
func (r *Runtime) Close() {
	r.closeOnce.Do(func() {
		r.cancel()
		r.conversations.Close()
		r.mcp.Close()
		_ = r.ledger.Close()
		r.recorder.Record(observability.Event{Name: observability.ApplicationStopped})
		_ = r.recorder.Close()
	})
}
