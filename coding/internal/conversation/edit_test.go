package conversation

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestManagerEditMessageRewritesCurrentSessionAndContinues(t *testing.T) {
	dataDir := t.TempDir()
	workspacePath := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	manager.streamFn = forkResponses("first answer", "old answer", "new answer")

	source, err := manager.Create(
		"Edit in place",
		workspacePath,
		ScopeProject,
		model,
		thinking,
		permission.ModeAutoEdit,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"first question", "edit this question"} {
		if err := manager.StartPromptWithFiles(source.ID, prompt, nil); err != nil {
			t.Fatal(err)
		}
		waitForSessionIdle(t, manager, source.ID)
	}
	runtime := mustRuntime(t, manager, source.ID)
	targetID := findMessageID(t, runtime.session.Entries(), true, "edit this question")

	updated, err := manager.EditMessage(source.ID, targetID, "edited question")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != source.ID || updated.WorkspacePath != source.WorkspacePath ||
		updated.ModelID != source.ModelID || updated.ThinkingLevel != source.ThinkingLevel ||
		updated.PermissionMode != source.PermissionMode || !updated.Running {
		t.Fatalf("edited summary = %+v, source = %+v", updated, source)
	}
	if updated.ForkedFromSessionID != source.ForkedFromSessionID ||
		updated.ForkedFromMessageID != source.ForkedFromMessageID {
		t.Fatalf("edit changed source relationship: %+v", updated)
	}
	if len(manager.List()) != 1 {
		t.Fatalf("editing created another session: %+v", manager.List())
	}
	waitForSessionIdle(t, manager, source.ID)

	entries := mustRuntime(t, manager, source.ID).session.Entries()
	editedID := findMessageID(t, entries, true, "edited question")
	if editedID == targetID {
		t.Fatal("edited message reused the discarded message ID")
	}
	findMessageID(t, entries, true, "first question")
	findMessageID(t, entries, false, "first answer")
	findMessageID(t, entries, false, "new answer")
	if hasMessageText(entries, "edit this question") || hasMessageText(entries, "old answer") {
		t.Fatalf("discarded history survived edit: %#v", entries)
	}

	manager.Close()
	restored := newTestManager(t, dataDir)
	if err := restored.EnsureLoaded(source.ID); err != nil {
		t.Fatal(err)
	}
	restoredRuntime := mustRuntime(t, restored, source.ID)
	restoredEntries := restoredRuntime.session.Entries()
	findMessageID(t, restoredEntries, true, "edited question")
	findMessageID(t, restoredEntries, false, "new answer")
	if hasMessageText(restoredEntries, "edit this question") || hasMessageText(restoredEntries, "old answer") {
		t.Fatalf("discarded history returned after reload: %#v", restoredEntries)
	}
}

func TestManagerEditMessageRejectsActiveSessionWithoutRewriting(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	started := make(chan struct{})
	release := make(chan struct{})
	manager.streamFn = func(
		_ context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		close(started)
		<-release
		return forkResponse(model, "answer", llm.Usage{}), nil
	}
	source, err := manager.Create("Active", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("model stream did not start")
	}
	runtime := mustRuntime(t, manager, source.ID)
	before, err := os.ReadFile(runtime.record.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.EditMessage(source.ID, "message", "edited"); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("EditMessage error = %v, want %v", err, ErrSessionActive)
	}
	after, err := os.ReadFile(runtime.record.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("active edit attempt rewrote transcript")
	}
	close(release)
	waitForSessionIdle(t, manager, source.ID)
}

func TestManagerEditMessageInvalidatesDiscardedBranchPoints(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	manager.streamFn = forkResponses("answer", "replacement answer")
	source, err := manager.Create("Parent", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)
	entries := mustRuntime(t, manager, source.ID).session.Entries()
	userID := findMessageID(t, entries, true, "question")
	assistantID := findMessageID(t, entries, false, "answer")
	child, err := manager.Fork(source.ID, ForkOptions{
		MessageID: assistantID,
		Mode:      transcript.ForkAfterAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.EditMessage(source.ID, userID, "replacement"); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)
	childSummary := mustRuntime(t, manager, child.ID).summary()
	if childSummary.ForkedFromSessionID != source.ID ||
		childSummary.ForkedFromMessageID != "" {
		t.Fatalf("discarded branch point was not invalidated: %+v", childSummary)
	}

	manager.Close()
	restored := newTestManager(t, dataDir)
	restoredChild := mustRuntime(t, restored, child.ID).summary()
	if restoredChild.ForkedFromSessionID != source.ID ||
		restoredChild.ForkedFromMessageID != "" {
		t.Fatalf("invalidated branch point was not restored: %+v", restoredChild)
	}
}

func TestManagerEditMessageKeepsRetainedBranchPoints(t *testing.T) {
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, t.TempDir())
	manager.streamFn = forkResponses("first answer", "second answer", "replacement answer")
	source, err := manager.Create("Parent", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"first question", "second question"} {
		if err := manager.StartPromptWithFiles(source.ID, prompt, nil); err != nil {
			t.Fatal(err)
		}
		waitForSessionIdle(t, manager, source.ID)
	}
	entries := mustRuntime(t, manager, source.ID).session.Entries()
	firstAssistantID := findMessageID(t, entries, false, "first answer")
	secondUserID := findMessageID(t, entries, true, "second question")
	child, err := manager.Fork(source.ID, ForkOptions{
		MessageID: firstAssistantID,
		Mode:      transcript.ForkAfterAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.EditMessage(source.ID, secondUserID, "replacement question"); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)

	childSummary := mustRuntime(t, manager, child.ID).summary()
	if childSummary.ForkedFromSessionID != source.ID ||
		childSummary.ForkedFromMessageID != firstAssistantID {
		t.Fatalf("retained branch point changed: %+v", childSummary)
	}
}

func TestManagerEditMessageRestoresTranscriptWhenIndexSaveFails(t *testing.T) {
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, t.TempDir())
	manager.streamFn = forkResponses("original answer")
	source, err := manager.Create("Restore", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "original question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)
	runtime := mustRuntime(t, manager, source.ID)
	targetID := findMessageID(t, runtime.session.Entries(), true, "original question")
	before, err := os.ReadFile(runtime.record.Transcript)
	if err != nil {
		t.Fatal(err)
	}

	manager.indexPath = runtime.record.Transcript + "/index.json"
	if _, err := manager.EditMessage(source.ID, targetID, "replacement question"); err == nil {
		t.Fatal("EditMessage succeeded when the session index could not be saved")
	}
	after, err := os.ReadFile(runtime.record.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed edit did not restore the original transcript")
	}
	entries := mustRuntime(t, manager, source.ID).session.Entries()
	findMessageID(t, entries, true, "original question")
	findMessageID(t, entries, false, "original answer")
	if hasMessageText(entries, "replacement question") {
		t.Fatal("failed edit changed the active runtime")
	}
}
