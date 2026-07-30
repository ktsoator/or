package conversation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/titlegen"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

type testTransport struct{ closed atomic.Bool }

func (*testTransport) Publish(Event)             {}
func (*testTransport) PublishAgent(engine.Event) {}
func (t *testTransport) Close()                  { t.closed.Store(true) }
func (*testTransport) Decide(context.Context, permission.ApprovalRequest) (permission.ApprovalResponse, error) {
	return permission.ApprovalResponse{Choice: permission.Reject}, nil
}
func (*testTransport) HasPendingApproval() bool { return false }
func (*testTransport) OpenBrowser(context.Context, tools.BrowserRequest) (tools.BrowserResult, error) {
	return tools.BrowserResult{Status: tools.BrowserFailed}, nil
}
func (*testTransport) Ask(context.Context, []tools.Question) ([]tools.Answer, error) {
	return nil, errors.New("no viewer")
}
func (*testTransport) HasPendingQuestion() bool { return false }

type recordingTransport struct {
	events chan Event
}

type titleGeneratorFunc func(context.Context, string) (titlegen.Result, error)

func (f titleGeneratorFunc) Generate(ctx context.Context, prompt string) (titlegen.Result, error) {
	return f(ctx, prompt)
}

func (t *recordingTransport) Publish(event Event)     { t.events <- event }
func (*recordingTransport) PublishAgent(engine.Event) {}
func (*recordingTransport) Close()                    {}
func (*recordingTransport) Decide(context.Context, permission.ApprovalRequest) (permission.ApprovalResponse, error) {
	return permission.ApprovalResponse{Choice: permission.Reject}, nil
}
func (*recordingTransport) HasPendingApproval() bool { return false }
func (*recordingTransport) OpenBrowser(context.Context, tools.BrowserRequest) (tools.BrowserResult, error) {
	return tools.BrowserResult{Status: tools.BrowserFailed}, nil
}
func (*recordingTransport) Ask(context.Context, []tools.Question) ([]tools.Answer, error) {
	return nil, errors.New("no viewer")
}
func (*recordingTransport) HasPendingQuestion() bool { return false }

func TestManagerCreatesAndRestoresProjectConversation(t *testing.T) {
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	model, thinking := testCatalogModel(t)

	manager := newTestManager(t, dataDir)
	created, err := manager.Create("  Refactor parser  ", projectDir, ScopeProject, model, thinking, permission.ModeAutoEdit)
	if err != nil {
		t.Fatal(err)
	}
	wantProjectDir, err := workspace.Validate(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "Refactor parser" || created.Scope != ScopeProject || created.WorkspaceKind != KindFolder {
		t.Fatalf("created summary = %+v", created)
	}
	if created.WorkspacePath != wantProjectDir || created.ModelProvider != model.Provider || created.ModelID != model.ID {
		t.Fatalf("created identity = %+v", created)
	}
	if created.PermissionMode != permission.ModeAutoEdit {
		t.Fatalf("created permission mode = %q, want %q", created.PermissionMode, permission.ModeAutoEdit)
	}

	restored := newTestManager(t, dataDir)
	items := restored.List()
	if len(items) != 1 {
		t.Fatalf("restored conversations = %d, want 1", len(items))
	}
	got := items[0]
	if got.ID != created.ID || got.Title != created.Title || got.WorkspacePath != created.WorkspacePath {
		t.Fatalf("restored summary = %+v, want %+v", got, created)
	}
	if got.PermissionMode != permission.ModeAutoEdit {
		t.Fatalf("restored permission mode = %q, want %q", got.PermissionMode, permission.ModeAutoEdit)
	}
	if !restored.UsesProvider(model.Provider) {
		t.Fatalf("restored manager does not report provider %q in use", model.Provider)
	}
}

func TestManagerRestoresConversationsLazily(t *testing.T) {
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	model, thinking := testCatalogModel(t)

	manager := newTestManager(t, dataDir)
	created, err := manager.Create(
		"Lazy session",
		projectDir,
		ScopeProject,
		model,
		thinking,
		permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := manager.Create(
		"Delete without loading",
		projectDir,
		ScopeProject,
		model,
		thinking,
		permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()

	var transports atomic.Int64
	restored := newTestManagerWithTransport(t, dataDir, func(string) Transport {
		transports.Add(1)
		return &testTransport{}
	})
	runtime, ok := restored.runtime(created.ID)
	if !ok {
		t.Fatal("restored conversation not found")
	}
	if runtime.session != nil || runtime.transport != nil {
		t.Fatal("restored conversation loaded before first use")
	}
	if got := restored.List(); len(got) != 2 {
		t.Fatalf("restored conversations = %+v", got)
	}
	if !restored.UsesProvider(model.Provider) {
		t.Fatalf("restored manager does not report provider %q in use", model.Provider)
	}
	renamed, err := restored.Rename(created.ID, "Renamed before loading")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "Renamed before loading" {
		t.Fatalf("renamed conversation = %+v", renamed)
	}
	updated, err := restored.UpdatePermissionMode(created.ID, permission.ModeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PermissionMode != permission.ModeReadOnly {
		t.Fatalf("updated conversation = %+v", updated)
	}
	if path, err := restored.WorkspacePath(created.ID); err != nil || path != created.WorkspacePath {
		t.Fatalf("WorkspacePath() = %q, %v; want %q", path, err, created.WorkspacePath)
	}
	if err := restored.Abort(created.ID); err != nil {
		t.Fatal(err)
	}
	if err := restored.Delete(deleted.ID); err != nil {
		t.Fatal(err)
	}
	if got := transports.Load(); got != 0 {
		t.Fatalf("transports created by metadata queries = %d, want 0", got)
	}
	if got := restored.List(); len(got) != 1 || got[0].ID != created.ID {
		t.Fatalf("conversations after metadata-only delete = %+v", got)
	}

	snapshot, err := restored.Snapshot(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Title != "Renamed before loading" {
		t.Fatalf("snapshot title = %q, want %q", snapshot.Title, "Renamed before loading")
	}
	if _, err := restored.Snapshot(created.ID); err != nil {
		t.Fatal(err)
	}
	if got := transports.Load(); got != 1 {
		t.Fatalf("transports after two snapshots = %d, want 1", got)
	}
	runtime, ok = restored.runtime(created.ID)
	if !ok || runtime.session == nil || runtime.transport == nil {
		t.Fatal("restored conversation was not loaded on first snapshot")
	}
}

func TestManagerDefersTranscriptErrorsUntilFirstUse(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)

	manager := newTestManager(t, dataDir)
	created, err := manager.Create(
		"Broken transcript",
		t.TempDir(),
		ScopeProject,
		model,
		thinking,
		permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	transcriptPath := runtime.record.Transcript
	manager.Close()
	if err := os.WriteFile(transcriptPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var transports []*testTransport
	restored := newTestManagerWithTransport(t, dataDir, func(string) Transport {
		transport := &testTransport{}
		transports = append(transports, transport)
		return transport
	})
	if got := restored.List(); len(got) != 1 || got[0].ID != created.ID {
		t.Fatalf("restored conversations = %+v", got)
	}
	if _, err := restored.Snapshot(created.ID); err == nil {
		t.Fatal("Snapshot succeeded with a malformed transcript")
	}
	if len(transports) != 1 || !transports[0].closed.Load() {
		t.Fatalf(
			"failed lazy-load transports = %d, closed = %v",
			len(transports),
			len(transports) == 1 && transports[0].closed.Load(),
		)
	}
	runtime, ok = restored.runtime(created.ID)
	if !ok || runtime.session != nil || runtime.transport != nil {
		t.Fatal("failed lazy load replaced the metadata-only conversation")
	}
}

func TestManagerRunReservationProtectsConversation(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := manager.reservePrompt(created.ID, "Inspect the parser", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.live.Load() {
		t.Fatal("runtime is not exposed as running")
	}
	if _, err := manager.reservePrompt(created.ID, "second", nil, nil); !errors.Is(err, engine.ErrBusy) {
		t.Fatalf("second reservation error = %v, want ErrBusy", err)
	}
	if err := manager.Delete(created.ID); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("Delete error = %v, want ErrSessionActive", err)
	}
	if _, err := manager.UpdateSettings(created.ID, model, thinking); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("UpdateSettings error = %v, want ErrSessionActive", err)
	}
	if _, err := manager.UpdatePermissionMode(created.ID, permission.ModeReadOnly); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("UpdatePermissionMode error = %v, want ErrSessionActive", err)
	}

	manager.finishRun(created.ID, runtime)
	if runtime.live.Load() {
		t.Fatal("runtime is still exposed as running after cleanup")
	}
	updated, err := manager.UpdatePermissionMode(created.ID, permission.ModeReadOnly)
	if err != nil || updated.PermissionMode != permission.ModeReadOnly {
		t.Fatalf("UpdatePermissionMode() = %+v, %v", updated, err)
	}
	if err := manager.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestManagerGeneratesTitleBeforeAssistantResponseCompletes(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}

	started := make(chan string, 1)
	release := make(chan struct{})
	manager.generateTitle = titleGeneratorFunc(func(ctx context.Context, prompt string) (titlegen.Result, error) {
		started <- prompt
		select {
		case <-release:
			return titlegen.Result{
				Title:    "Inspect parser behavior",
				Provider: "utility-provider",
				Model:    "utility-model",
			}, nil
		case <-ctx.Done():
			return titlegen.Result{}, ctx.Err()
		}
	})
	events := make(chan Event, 4)
	runtime.transport = &recordingTransport{events: events}

	manager.handleSessionEvent(created.ID, runtime, engine.Event{
		Type: engine.UserMessageCompleted,
		Text: "Inspect the parser behavior",
	})
	select {
	case prompt := <-started:
		if prompt != "Inspect the parser behavior" {
			t.Fatalf("title prompt = %q", prompt)
		}
	case <-time.After(time.Second):
		t.Fatal("title generation did not start after the user message")
	}

	manager.handleSessionEvent(created.ID, runtime, engine.Event{
		Type:          engine.MessageCompleted,
		FinalResponse: false,
	})
	close(release)

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-events:
			changed, ok := event.(TitleChanged)
			if !ok {
				continue
			}
			if changed.Title != "Inspect parser behavior" || changed.AITitle != "Inspect parser behavior" {
				t.Fatalf("title event = %+v", changed)
			}
			goto titlePublished
		case <-deadline:
			t.Fatal("title was not published after an interrupted response")
		}
	}

titlePublished:

	if got := manager.List()[0]; got.Title != "Inspect parser behavior" ||
		got.AITitle != "Inspect parser behavior" ||
		got.TitleGeneration.Status != TitleGenerationSucceeded ||
		got.TitleGeneration.Provider != "utility-provider" ||
		got.TitleGeneration.Model != "utility-model" {
		t.Fatalf("conversation summary = %+v", got)
	}
}

func TestManagerTitleFailureKeepsPromptFallbackAndExposesState(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	manager.mu.Lock()
	runtime.record.Title = titleFromPrompt("Investigate flaky title generation")
	runtime.record.AutoTitle = false
	manager.mu.Unlock()
	manager.generateTitle = titleGeneratorFunc(func(context.Context, string) (titlegen.Result, error) {
		return titlegen.Result{Provider: "utility-provider", Model: "utility-model"}, errors.New("secret upstream detail")
	})
	events := make(chan Event, 4)
	runtime.transport = &recordingTransport{events: events}

	manager.handleSessionEvent(created.ID, runtime, engine.Event{
		Type: engine.UserMessageCompleted,
		Text: "Investigate flaky title generation",
	})

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-events:
			changed, ok := event.(TitleGenerationChanged)
			if !ok || changed.Generation.Status == TitleGenerationGenerating {
				continue
			}
			if changed.Generation.Status != TitleGenerationFailed ||
				changed.Generation.ErrorCode != titlegen.CodeRequestFailed ||
				changed.Generation.Error == "secret upstream detail" {
				t.Fatalf("title generation state = %+v", changed.Generation)
			}
			goto failurePublished
		case <-deadline:
			t.Fatal("title failure was not published")
		}
	}

failurePublished:
	got := manager.List()[0]
	if got.Title != "Investigate flaky title generation" || got.AITitle != "" {
		t.Fatalf("fallback title was replaced: %+v", got)
	}
}

func TestManagerCustomTitleWinsOverLateUtilityTitle(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	manager.generateTitle = titleGeneratorFunc(func(ctx context.Context, _ string) (titlegen.Result, error) {
		close(started)
		select {
		case <-release:
			return titlegen.Result{Title: "Late generated title"}, nil
		case <-ctx.Done():
			return titlegen.Result{}, ctx.Err()
		}
	})
	runtime.transport = &recordingTransport{events: make(chan Event, 8)}
	manager.handleSessionEvent(created.ID, runtime, engine.Event{
		Type: engine.UserMessageCompleted,
		Text: "Generate a title",
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("title generation did not start")
	}
	if _, err := manager.Rename(created.ID, "My custom title"); err != nil {
		t.Fatal(err)
	}
	close(release)
	manager.tasks.Wait()

	got := manager.List()[0]
	if got.Title != "My custom title" || got.CustomTitle != "My custom title" || got.AITitle != "" {
		t.Fatalf("late generated title won: %+v", got)
	}
}

func TestManagerDeleteRemovesScratchWorkspaceAndSessionFiles(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("Scratch", "", ScopeChat, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	transport := runtime.transport.(*testTransport)
	transcriptPath := runtime.record.Transcript
	detailsPath := detailsFile(transcriptPath)
	if err := os.WriteFile(transcriptPath, []byte("transcript"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(detailsPath, []byte("details"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := manager.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{created.WorkspacePath, transcriptPath, detailsPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still exists or stat failed: %v", path, err)
		}
	}
	if len(manager.List()) != 0 {
		t.Fatalf("conversations after delete = %+v", manager.List())
	}
	if !transport.closed.Load() {
		t.Fatal("deleted conversation transport was not closed")
	}
	assertIndexDoesNotContain(t, manager.indexPath, created.ID)
}

func TestManagerClosesTransportWhenCreateRollsBack(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	var createdTransports []*testTransport
	manager := newTestManagerWithTransport(t, dataDir, func(string) Transport {
		transport := &testTransport{}
		createdTransports = append(createdTransports, transport)
		return transport
	})
	if err := os.Mkdir(manager.indexPath+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Create("Rollback", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk); err == nil {
		t.Fatal("Create succeeded with a directory blocking the index temp file")
	}
	if len(createdTransports) != 1 || !createdTransports[0].closed.Load() {
		t.Fatalf("rolled-back transports = %d, closed = %v", len(createdTransports), len(createdTransports) == 1 && createdTransports[0].closed.Load())
	}
}

func TestManagerDeleteRestoresFilesWhenIndexWriteFails(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("Rollback", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	transport := runtime.transport.(*testTransport)
	transcriptPath := runtime.record.Transcript
	if err := os.WriteFile(transcriptPath, []byte("transcript"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockingPath := manager.indexPath + ".tmp"
	if err := os.Mkdir(blockingPath, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := manager.Delete(created.ID); err == nil {
		t.Fatal("Delete succeeded with a directory blocking the index temp file")
	}
	if _, ok := manager.runtime(created.ID); !ok {
		t.Fatal("conversation was not restored after failed delete")
	}
	if transport.closed.Load() {
		t.Fatal("failed delete closed the restored conversation transport")
	}
	if _, err := os.Stat(transcriptPath); err != nil {
		t.Fatalf("transcript was not restored: %v", err)
	}
	matches, err := filepath.Glob(transcriptPath + ".deleted-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("staged transcript remains after rollback: %v", matches)
	}
}

func TestManagerPermissionModeRollsBackWhenIndexWriteFails(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("Permissions", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manager.indexPath+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.UpdatePermissionMode(created.ID, permission.ModeAutoEdit); err == nil {
		t.Fatal("UpdatePermissionMode succeeded with a directory blocking the index temp file")
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("conversation missing after failed permission update")
	}
	if got := runtime.summary().PermissionMode; got != permission.ModeAsk {
		t.Fatalf("permission mode after rollback = %q, want %q", got, permission.ModeAsk)
	}
}

func TestManagerCloseIsIdempotentAndRejectsNewWork(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	transport := runtime.transport.(*testTransport)

	manager.Close()
	manager.Close()

	if !transport.closed.Load() {
		t.Fatal("manager close did not close the conversation transport")
	}
	if err := manager.StartPrompt(created.ID, "after shutdown"); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("StartPrompt error = %v, want ErrManagerClosed", err)
	}
	if _, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Create error = %v, want ErrManagerClosed", err)
	}
}

func newTestManager(t *testing.T, dataDir string) *Manager {
	t.Helper()
	return newTestManagerWithTransport(t, dataDir, func(string) Transport {
		return &testTransport{}
	})
}

func newTestManagerWithTransport(
	t *testing.T,
	dataDir string,
	newTransport NewTransport,
) *Manager {
	t.Helper()
	home := filepath.Join(dataDir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	ledger, err := usage.NewStore(filepath.Join(dataDir, "usage", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := workspace.NewRegistry(filepath.Join(dataDir, "sessions", "workspaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(context.Background(), Options{
		DataDir:      dataDir,
		Usage:        ledger,
		Workspaces:   workspaces,
		NewTransport: newTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		manager.Close()
	})
	return manager
}

func testCatalogModel(t *testing.T) (llm.Model, llm.ModelThinkingLevel) {
	t.Helper()
	for _, provider := range llm.GetProviders() {
		models := llm.GetModels(provider)
		if len(models) == 0 {
			continue
		}
		levels := llm.SupportedThinkingLevels(models[0])
		if len(levels) == 0 {
			continue
		}
		return models[0], levels[0]
	}
	t.Fatal("built-in model catalog is empty")
	return llm.Model{}, ""
}

func assertIndexDoesNotContain(t *testing.T, path, value string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), value) {
		t.Fatalf("index %s still contains %q", path, value)
	}
}
