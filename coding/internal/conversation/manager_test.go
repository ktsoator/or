package conversation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/usage"
	"github.com/ktsoator/or/coding/internal/workspace"
	"github.com/ktsoator/or/llm"
)

type testTransport struct{}

func (*testTransport) Publish(Event)             {}
func (*testTransport) PublishAgent(engine.Event) {}
func (*testTransport) Decide(context.Context, permission.ApprovalRequest) (permission.ApprovalResponse, error) {
	return permission.ApprovalResponse{Choice: permission.Reject}, nil
}
func (*testTransport) HasPendingApproval() bool { return false }

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

func TestManagerRunReservationProtectsConversation(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}

	runtime, err := manager.BeginPrompt(created.ID, "Inspect the parser", false)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.Running() {
		t.Fatal("runtime is not exposed as running")
	}
	if _, err := manager.BeginPrompt(created.ID, "second", false); !errors.Is(err, engine.ErrBusy) {
		t.Fatalf("second BeginPrompt error = %v, want ErrBusy", err)
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

	manager.EndRun(created.ID)
	if runtime.Running() {
		t.Fatal("runtime is still exposed as running after EndRun")
	}
	updated, err := manager.UpdatePermissionMode(created.ID, permission.ModeReadOnly)
	if err != nil || updated.PermissionMode != permission.ModeReadOnly {
		t.Fatalf("UpdatePermissionMode() = %+v, %v", updated, err)
	}
	if err := manager.Delete(created.ID); err != nil {
		t.Fatal(err)
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
	runtime, ok := manager.Get(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
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
	assertIndexDoesNotContain(t, manager.indexPath, created.ID)
}

func TestManagerDeleteRestoresFilesWhenIndexWriteFails(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	created, err := manager.Create("Rollback", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	runtime, ok := manager.Get(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
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
	if _, ok := manager.Get(created.ID); !ok {
		t.Fatal("conversation was not restored after failed delete")
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
	runtime, ok := manager.Get(created.ID)
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

	manager.Close()
	manager.Close()

	if _, err := manager.BeginPrompt(created.ID, "after shutdown", false); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("BeginPrompt error = %v, want ErrManagerClosed", err)
	}
	if _, err := manager.Create("", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Create error = %v, want ErrManagerClosed", err)
	}
}

func newTestManager(t *testing.T, dataDir string) *Manager {
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
		NewTransport: func(string) Transport { return &testTransport{} },
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
