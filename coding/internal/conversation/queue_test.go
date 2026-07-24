package conversation

import (
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
	runtime, ok := manager.Get(created.ID)
	if !ok {
		t.Fatal("created conversation not found")
	}

	runtime.running.Store(true)
	if !runtime.Queue(QueuedMessage{
		ID:       "followup",
		Delivery: DeliveryFollowUp,
		Text:     "same content",
	}) {
		t.Fatal("follow-up was not queued")
	}
	if !runtime.Queue(QueuedMessage{
		ID:       "steer",
		Delivery: DeliverySteer,
		Text:     "same content",
	}) {
		t.Fatal("steer was not queued")
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
