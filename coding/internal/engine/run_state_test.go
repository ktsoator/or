package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestRunExecutionStateOwnsRunMechanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstErr := errors.New("first persistence failure")
	secondErr := errors.New("second persistence failure")
	var state runExecutionState

	state.begin(ctx)
	if state.runContext() != ctx {
		t.Fatal("run context was not retained")
	}
	state.recordPersistenceError(nil)
	state.recordPersistenceError(firstErr)
	state.recordPersistenceError(secondErr)
	if !errors.Is(state.persistenceError(), firstErr) {
		t.Fatalf("persistence error = %v, want first error", state.persistenceError())
	}

	var claims atomic.Int32
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if state.claimAutoCompaction() {
				claims.Add(1)
			}
		}()
	}
	wait.Wait()
	if claims.Load() != 1 {
		t.Fatalf("auto-compaction claims = %d, want 1", claims.Load())
	}

	state.end()
	if state.runContext() != nil || state.persistenceError() != nil {
		t.Fatal("end did not clear run execution state")
	}
	state.begin(context.Background())
	if !state.claimAutoCompaction() {
		t.Fatal("new run did not reset auto-compaction claim")
	}
}

func TestSessionContextWindowPolicyFollowsAgentModel(t *testing.T) {
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "small", ContextWindow: 100},
		Tools: []tools.Tool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !session.shouldAutoCompact(80) {
		t.Fatal("small model did not trigger proactive compaction")
	}
	message := llm.AssistantMessage{
		StopReason: llm.StopReasonLength,
		Usage:      llm.Usage{Input: 99},
	}
	session.agent.SetMessages([]agent.AgentMessage{agent.FromLLM(&message)})
	if !session.trailingContextOverflow() {
		t.Fatal("small model did not detect a full-window length stop")
	}

	session.SetModel(llm.Model{Provider: "test", ID: "large", ContextWindow: 1_000})
	if session.shouldAutoCompact(80) {
		t.Fatal("proactive compaction used the previous model window")
	}
	if session.trailingContextOverflow() {
		t.Fatal("overflow detection used the previous model window")
	}
}

func TestAutoCompactClaimsOneEffectiveAttemptPerRun(t *testing.T) {
	ctx := context.Background()
	store := &transcript.Memory{}
	if err := store.Append(ctx, seededTurns(6)...); err != nil {
		t.Fatal(err)
	}
	compactor := &recordingCompactor{}
	session, err := New(ctx, Options{
		Model:     llm.Model{Provider: "test", ID: "model", ContextWindow: 400},
		Tools:     []tools.Tool{},
		Store:     store,
		Compactor: compactor,
	})
	if err != nil {
		t.Fatal(err)
	}
	session.execution.begin(ctx)
	defer session.execution.end()

	var compacted atomic.Int32
	var wait sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			didCompact, err := session.autoCompact(ctx)
			if didCompact {
				compacted.Add(1)
			}
			errs <- err
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if compacted.Load() != 1 || len(compactor.requests) != 1 {
		t.Fatalf("compactions = %d, requests = %d; want 1/1", compacted.Load(), len(compactor.requests))
	}
}
