package conversation

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestManagerForkAfterAssistantCreatesIndependentSession(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	manager.streamFn = forkResponses("source answer")

	source, err := manager.Create(
		"Source title",
		t.TempDir(),
		ScopeProject,
		model,
		thinking,
		permission.ModeAutoEdit,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "source question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)

	sourceRuntime, _ := manager.runtime(source.ID)
	assistantID := findMessageID(t, sourceRuntime.session.Entries(), false, "source answer")
	before, err := os.ReadFile(sourceRuntime.record.Transcript)
	if err != nil {
		t.Fatal(err)
	}

	child, err := manager.Fork(source.ID, ForkOptions{
		MessageID: assistantID,
		Mode:      transcript.ForkAfterAssistant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.ID == source.ID || child.Running {
		t.Fatalf("forked summary = %+v", child)
	}
	if child.Title != source.Title || child.WorkspacePath != source.WorkspacePath ||
		child.Scope != source.Scope || child.WorkspaceKind != source.WorkspaceKind ||
		child.ModelProvider != source.ModelProvider || child.ModelID != source.ModelID ||
		child.ThinkingLevel != source.ThinkingLevel || child.PermissionMode != source.PermissionMode {
		t.Fatalf("fork did not inherit source settings: source=%+v child=%+v", source, child)
	}
	if child.ForkedFromSessionID != source.ID || child.ForkedFromMessageID != assistantID {
		t.Fatalf("fork source metadata = %+v", child)
	}
	after, err := os.ReadFile(sourceRuntime.record.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("fork modified the source transcript")
	}

	childRuntime, ok := manager.runtime(child.ID)
	if !ok {
		t.Fatal("forked session is missing")
	}
	childEntries := childRuntime.session.Entries()
	if len(childEntries) != len(sourceRuntime.session.Entries()) {
		t.Fatalf("fork entries = %d, source entries = %d", len(childEntries), len(sourceRuntime.session.Entries()))
	}
}

func TestManagerForkBeforeUserContinuesWithReplacement(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	manager.streamFn = forkResponses("first answer", "second answer", "branch answer")

	source, err := manager.Create("Edit source", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"first question", "edit this question"} {
		if err := manager.StartPromptWithFiles(source.ID, prompt, nil); err != nil {
			t.Fatal(err)
		}
		waitForSessionIdle(t, manager, source.ID)
	}

	sourceRuntime, _ := manager.runtime(source.ID)
	targetID := findMessageID(t, sourceRuntime.session.Entries(), true, "edit this question")
	child, err := manager.Fork(source.ID, ForkOptions{
		MessageID:       targetID,
		Mode:            transcript.ForkBeforeUser,
		ReplacementText: "edited question",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !child.Running {
		t.Fatalf("forked edit should start its response: %+v", child)
	}
	waitForSessionIdle(t, manager, child.ID)

	childRuntime, _ := manager.runtime(child.ID)
	childEntries := childRuntime.session.Entries()
	findMessageID(t, childEntries, true, "edited question")
	findMessageID(t, childEntries, false, "branch answer")
	if hasMessageText(sourceRuntime.session.Entries(), "edited question") ||
		!hasMessageText(sourceRuntime.session.Entries(), "edit this question") {
		t.Fatal("editing a fork changed the source transcript")
	}
	if childRuntime.record.UsageBackfillOffset >= len(childEntries) {
		t.Fatalf("fork response was not appended after usage offset: offset=%d entries=%d", childRuntime.record.UsageBackfillOffset, len(childEntries))
	}
}

func TestManagerForkRejectsActiveSessionAndInvalidBoundary(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	release := make(chan struct{})
	started := make(chan struct{})
	manager.streamFn = func(_ context.Context, model llm.Model, _ llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		close(started)
		<-release
		return forkResponse(model, "answer", llm.Usage{}), nil
	}

	source, err := manager.Create("Active source", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
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
	if _, err := manager.Fork(source.ID, ForkOptions{
		MessageID: "anything",
		Mode:      transcript.ForkAfterAssistant,
	}); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("Fork error = %v, want %v", err, ErrSessionActive)
	}
	close(release)
	waitForSessionIdle(t, manager, source.ID)

	if _, err := manager.Fork(source.ID, ForkOptions{
		MessageID: "missing",
		Mode:      transcript.ForkAfterAssistant,
	}); !errors.Is(err, transcript.ErrForkMessageNotFound) {
		t.Fatalf("Fork error = %v, want %v", err, transcript.ErrForkMessageNotFound)
	}
	userID := findMessageID(t, mustRuntime(t, manager, source.ID).session.Entries(), true, "question")
	if _, err := manager.Fork(source.ID, ForkOptions{
		MessageID: userID,
		Mode:      transcript.ForkAfterAssistant,
	}); !errors.Is(err, transcript.ErrInvalidForkBoundary) {
		t.Fatalf("Fork error = %v, want %v", err, transcript.ErrInvalidForkBoundary)
	}
}

func TestManagerForkedScratchWorkspaceLivesUntilLastSessionIsDeleted(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	manager.streamFn = forkResponses("answer")

	source, err := manager.Create("Scratch source", "", ScopeChat, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)
	assistantID := findMessageID(t, mustRuntime(t, manager, source.ID).session.Entries(), false, "answer")
	child, err := manager.Fork(source.ID, ForkOptions{MessageID: assistantID, Mode: transcript.ForkAfterAssistant})
	if err != nil {
		t.Fatal(err)
	}
	if child.WorkspacePath != source.WorkspacePath {
		t.Fatalf("fork workspace = %q, source workspace = %q", child.WorkspacePath, source.WorkspacePath)
	}

	if err := manager.Delete(source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source.WorkspacePath); err != nil {
		t.Fatalf("shared scratch workspace removed with first session: %v", err)
	}
	if err := manager.Delete(child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source.WorkspacePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scratch workspace remains after last session: %v", err)
	}
}

func TestManagerForkDoesNotBackfillCopiedUsageAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	model, thinking := testCatalogModel(t)
	manager := newTestManager(t, dataDir)
	manager.streamFn = forkResponsesWithUsage(llm.Usage{Input: 4, Output: 6, TotalTokens: 10}, "answer")

	source, err := manager.Create("Usage source", t.TempDir(), ScopeProject, model, thinking, permission.ModeAsk)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StartPromptWithFiles(source.ID, "question", nil); err != nil {
		t.Fatal(err)
	}
	waitForSessionIdle(t, manager, source.ID)
	assistantID := findMessageID(t, mustRuntime(t, manager, source.ID).session.Entries(), false, "answer")
	child, err := manager.Fork(source.ID, ForkOptions{MessageID: assistantID, Mode: transcript.ForkAfterAssistant})
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.usage.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 1 {
		t.Fatalf("usage requests before restart = %d, want 1", report.Total.Requests)
	}
	manager.Close()

	restored := newTestManager(t, dataDir)
	if _, err := restored.Snapshot(child.ID); err != nil {
		t.Fatal(err)
	}
	report, err = restored.usage.Report(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Requests != 1 || report.Total.TotalTokens != 10 {
		t.Fatalf("usage after fork restore = %+v, want one 10-token request", report.Total)
	}
}

func forkResponses(texts ...string) agent.StreamFn {
	return forkResponsesWithUsage(llm.Usage{}, texts...)
}

func forkResponsesWithUsage(usage llm.Usage, texts ...string) agent.StreamFn {
	var index atomic.Int64
	return func(_ context.Context, model llm.Model, _ llm.Context, _ llm.StreamOptions) (<-chan llm.Event, error) {
		current := int(index.Add(1)) - 1
		if current >= len(texts) {
			return nil, errors.New("unexpected model request")
		}
		return forkResponse(model, texts[current], usage), nil
	}
}

func forkResponse(model llm.Model, text string, usage llm.Usage) <-chan llm.Event {
	message := llm.NewAssistantMessage(model)
	message.Content = []llm.AssistantContent{&llm.TextContent{Text: text}}
	message.StopReason = llm.StopReasonStop
	message.Usage = usage
	events := make(chan llm.Event, 1)
	events <- llm.Event{Type: llm.EventDone, Message: &message}
	close(events)
	return events
}

func waitForSessionIdle(t *testing.T, manager *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime := mustRuntime(t, manager, id)
		if !runtime.running.Load() && !runtime.live.Load() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("session %s did not become idle", id)
}

func mustRuntime(t *testing.T, manager *Manager, id string) *sessionRuntime {
	t.Helper()
	runtime, ok := manager.runtime(id)
	if !ok {
		t.Fatalf("session %s is missing", id)
	}
	return runtime
}

func findMessageID(t *testing.T, entries []transcript.Entry, user bool, text string) string {
	t.Helper()
	for _, entry := range entries {
		if entry.Type != transcript.MessageEntry || entry.Message == nil {
			continue
		}
		message, ok := agent.ToLLM(entry.Message)
		if !ok || messageText(message) != text {
			continue
		}
		_, isUser := message.(*llm.UserMessage)
		if isUser == user {
			return entry.ID
		}
	}
	t.Fatalf("message %q not found", text)
	return ""
}

func hasMessageText(entries []transcript.Entry, text string) bool {
	for _, entry := range entries {
		message, ok := agent.ToLLM(entry.Message)
		if ok && messageText(message) == text {
			return true
		}
	}
	return false
}

func messageText(message llm.Message) string {
	switch typed := message.(type) {
	case *llm.UserMessage:
		for _, content := range typed.Content {
			if block, ok := content.(*llm.TextContent); ok && block != nil {
				return block.Text
			}
		}
	case *llm.AssistantMessage:
		for _, content := range typed.Content {
			if block, ok := content.(*llm.TextContent); ok && block != nil {
				return block.Text
			}
		}
	}
	return ""
}
