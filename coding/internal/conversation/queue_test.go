package conversation

import (
	"errors"
	"os"
	"testing"

	"github.com/ktsoator/or/coding/internal/engine"
	"github.com/ktsoator/or/coding/internal/permission"
)

func TestHandleSessionEventConsumesPendingByQueueHandle(t *testing.T) {
	manager := newTestManager(t, t.TempDir())
	model, thinking := testCatalogModel(t)
	created, err := manager.Create(
		"Queue identity",
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

	runtime.running.Store(true)
	if err := manager.QueueMessage(created.ID, QueuedMessage{
		ID:       "followup",
		Delivery: DeliveryFollowUp,
		Text:     "same content",
	}); err != nil {
		t.Fatalf("queue follow-up: %v", err)
	}
	if err := manager.QueueMessage(created.ID, QueuedMessage{
		ID:       "steer",
		Delivery: DeliverySteer,
		Text:     "same content",
	}); err != nil {
		t.Fatalf("queue steer: %v", err)
	}

	runtime.pendingMu.Lock()
	steerHandle := runtime.pending[1].Handle
	runtime.pendingMu.Unlock()

	events := make(chan Event, 1)
	runtime.transport = &recordingTransport{events: events}
	manager.handleSessionEvent(created.ID, runtime, engine.Event{
		Type:        engine.UserMessageCompleted,
		Text:        "same content",
		QueueHandle: steerHandle,
	})

	event := <-events
	accepted, ok := event.(MessageAccepted)
	if !ok {
		t.Fatalf("event type = %T, want MessageAccepted", event)
	}
	if accepted.ID != "steer" || accepted.Delivery != DeliverySteer || accepted.Queued {
		t.Fatalf("accepted event = %#v, want completed steer", accepted)
	}

	runtime.pendingMu.Lock()
	defer runtime.pendingMu.Unlock()
	if len(runtime.pending) != 1 || runtime.pending[0].ID != "followup" {
		t.Fatalf("pending messages = %#v, want only follow-up", runtime.pending)
	}
}

func TestManagerQueueCommandsOwnRuntimeValidation(t *testing.T) {
	manager := newTestManager(t, t.TempDir())
	model, thinking := testCatalogModel(t)
	created, err := manager.Create(
		"Queue commands",
		t.TempDir(),
		ScopeProject,
		model,
		thinking,
		permission.ModeAsk,
	)
	if err != nil {
		t.Fatal(err)
	}
	message := QueuedMessage{
		ID:       "queued",
		Delivery: DeliveryFollowUp,
		Text:     "next",
	}
	if err := manager.QueueMessage(created.ID, message); !errors.Is(err, ErrSessionNotRunning) {
		t.Fatalf("idle QueueMessage error = %v, want ErrSessionNotRunning", err)
	}

	runtime, ok := manager.runtime(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}
	runtime.running.Store(true)
	if err := manager.QueueMessage(created.ID, message); err != nil {
		t.Fatal(err)
	}
	if err := manager.DequeueMessage(created.ID, "missing"); !errors.Is(err, ErrQueuedMessageNotFound) {
		t.Fatalf("missing DequeueMessage error = %v, want ErrQueuedMessageNotFound", err)
	}
	if err := manager.DequeueMessage(created.ID, message.ID); err != nil {
		t.Fatal(err)
	}

	snapshot, err := manager.Snapshot(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Queue) != 0 {
		t.Fatalf("snapshot queue = %#v, want empty", snapshot.Queue)
	}
	if _, err := manager.Snapshot("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing Snapshot error = %v, want os.ErrNotExist", err)
	}
}
