package engine

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ktsoator/or/coding/internal/observability"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type memoryRecorder struct {
	mu     sync.Mutex
	events []observability.Event
}

func (r *memoryRecorder) Record(event observability.Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (*memoryRecorder) Close() error { return nil }

func (r *memoryRecorder) snapshot() []observability.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]observability.Event(nil), r.events...)
}

func TestRunObservabilityUsesDurableRunIdentity(t *testing.T) {
	recorder := &memoryRecorder{}
	session, err := New(context.Background(), Options{
		SessionID: "session-1",
		Recorder:  recorder,
		Model:     llm.Model{Provider: "test", ID: "model"},
		Tools:     []tools.Tool{},
		Store:     &transcript.Memory{},
		StreamFn:  fixedResponse("answer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}

	events := recorder.snapshot()
	if len(events) != 2 || events[0].Name != observability.RunStarted ||
		events[1].Name != observability.RunCompleted {
		t.Fatalf("observability events = %#v", events)
	}
	if events[0].RunID == "" || events[0].RunID != events[1].RunID ||
		events[0].SessionID != "session-1" || events[1].Status != "completed" {
		t.Fatalf("run correlation = %#v", events)
	}
	entries := session.Entries()
	runEntry := entries[len(entries)-1]
	if runEntry.Type != transcript.RunEntry || runEntry.ID != events[0].RunID {
		t.Fatalf("run entry = %#v, events = %#v", runEntry, events)
	}
	if events[1].Duration < 0 || events[1].ErrorCode != "" {
		t.Fatalf("completed event = %#v", events[1])
	}
}

func TestRunObservabilityClassifiesFailureWithoutErrorText(t *testing.T) {
	recorder := &memoryRecorder{}
	sensitive := "provider failed with secret payload"
	zeroRetries := 0
	session, err := New(context.Background(), Options{
		SessionID:  "session-2",
		Recorder:   recorder,
		Model:      llm.Model{Provider: "test", ID: "model"},
		Tools:      []tools.Tool{},
		MaxRetries: &zeroRetries,
		StreamFn: func(
			context.Context,
			llm.Model,
			llm.Context,
			llm.StreamOptions,
		) (<-chan llm.Event, error) {
			return nil, errors.New(sensitive)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err == nil || err.Error() != sensitive {
		t.Fatalf("prompt error = %v", err)
	}

	events := recorder.snapshot()
	if len(events) != 2 || events[1].Name != observability.RunFailed ||
		events[1].Status != "failed" || events[1].ErrorCode != "run_failed" {
		t.Fatalf("failure events = %#v", events)
	}
}

var _ observability.Recorder = (*memoryRecorder)(nil)
