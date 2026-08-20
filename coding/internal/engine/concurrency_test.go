package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestSessionRejectsConcurrentRun(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRun := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRun)
	stream := func(
		ctx context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		started <- struct{}{}
		select {
		case <-release:
			return assistantEvents(model, "done"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	session, err := New(context.Background(), Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Tools:    []tools.Tool{},
		StreamFn: stream,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- session.Prompt(context.Background(), "first") }()
	awaitSignal(t, started, "first provider request")

	if err := session.Prompt(context.Background(), "second"); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent Prompt() error = %v, want ErrBusy", err)
	}
	releaseRun()
	if err := awaitResult(t, firstDone, "first run"); err != nil {
		t.Fatalf("first Prompt() error = %v", err)
	}
}

func TestSessionsRunIndependently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseRuns := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRuns)
	stream := func(
		ctx context.Context,
		model llm.Model,
		_ llm.Context,
		_ llm.StreamOptions,
	) (<-chan llm.Event, error) {
		started <- struct{}{}
		select {
		case <-release:
			return assistantEvents(model, "done"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	newSession := func() *Session {
		session, err := New(context.Background(), Options{
			Model:    llm.Model{Provider: "test", ID: "model"},
			Tools:    []tools.Tool{},
			StreamFn: stream,
		})
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	first := newSession()
	second := newSession()

	results := make(chan error, 2)
	go func() { results <- first.Prompt(context.Background(), "first") }()
	go func() { results <- second.Prompt(context.Background(), "second") }()
	awaitSignal(t, started, "first session provider request")
	awaitSignal(t, started, "second session provider request")
	releaseRuns()

	for index := 0; index < 2; index++ {
		if err := awaitResult(t, results, "independent session run"); err != nil {
			t.Fatalf("session Prompt() error = %v", err)
		}
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, subject string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", subject)
	}
}

func awaitResult(t *testing.T, result <-chan error, subject string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", subject)
		return nil
	}
}
