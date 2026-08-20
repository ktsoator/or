package engine

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ktsoator/or/coding/internal/transcript"
)

func TestLifecycleCoordinatorOwnsInitialProviderFailure(t *testing.T) {
	coordinator := &lifecycleCoordinator{}
	startedAt := lifecycleTestTime()
	started := coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
	if started.runID != "run-1" || started.turnID != "turn-1" {
		t.Fatalf("started run = %#v", started)
	}

	checkpoint := coordinator.beginStepCheckpointWithID(
		1,
		"step-1",
		startedAt.Add(time.Second),
	)
	request := coordinator.attachProviderRequestWithID("request-1")
	if request.runID != "run-1" || request.turnID != "turn-1" ||
		request.stepID != "step-1" || request.requestID != "request-1" {
		t.Fatalf("request correlation = %#v", request)
	}
	assertPositionedLifecycleTypes(t, checkpoint.entries, []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
	})
	coordinator.commitStepCheckpoint("step-1", checkpoint.pendingCount)

	completed := coordinator.completeStep(
		2,
		startedAt.Add(2*time.Second),
		"failed",
		"provider_request_failed",
	)
	if completed.step.requestID != "request-1" || completed.status != "failed" {
		t.Fatalf("completed step = %#v", completed)
	}
	terminal := coordinator.finishRun(
		2,
		startedAt.Add(3*time.Second),
		errors.New("provider failed"),
		nil,
	)
	assertPositionedLifecycleTypes(t, terminal.entries, []transcript.EntryType{
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	})
	if terminal.entries[0].entry.Lifecycle.Status != transcript.LifecycleFailed ||
		terminal.entries[0].entry.Lifecycle.Reason != "provider_request_failed" ||
		terminal.status != transcript.LifecycleFailed || terminal.reason != "run_failed" {
		t.Fatalf("provider failure terminal = %#v", terminal)
	}
}

func TestLifecycleCoordinatorSeparatesFollowUpFromSteering(t *testing.T) {
	startedAt := lifecycleTestTime()
	t.Run("follow-up starts a new turn", func(t *testing.T) {
		coordinator := &lifecycleCoordinator{}
		coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
		first := coordinator.beginStepCheckpointWithID(1, "step-1", startedAt)
		coordinator.attachProviderRequestWithID("request-1")
		coordinator.commitStepCheckpoint("step-1", first.pendingCount)
		coordinator.completeStep(2, startedAt.Add(time.Second), "completed", "")

		followUp := coordinator.claimFollowUp(2, startedAt.Add(2*time.Second))
		if followUp.previousTurnID != "turn-1" || followUp.nextTurnID == "turn-1" {
			t.Fatalf("follow-up transition = %#v", followUp)
		}
		second := coordinator.beginStepCheckpointWithID(
			3,
			"step-2",
			startedAt.Add(3*time.Second),
		)
		if second.step.lifecycleTurnID != followUp.nextTurnID {
			t.Fatalf("follow-up step = %#v, transition %#v", second.step, followUp)
		}
		assertPositionedLifecycleTypes(t, second.entries, []transcript.EntryType{
			transcript.StepEndEntry,
			transcript.TurnEndEntry,
			transcript.TurnStartEntry,
			transcript.StepStartEntry,
		})
	})

	t.Run("steering stays in the active turn", func(t *testing.T) {
		coordinator := &lifecycleCoordinator{}
		coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
		first := coordinator.beginStepCheckpointWithID(1, "step-1", startedAt)
		coordinator.attachProviderRequestWithID("request-1")
		coordinator.commitStepCheckpoint("step-1", first.pendingCount)
		coordinator.completeStep(2, startedAt.Add(time.Second), "completed", "")

		second := coordinator.beginStepCheckpointWithID(
			3,
			"step-2",
			startedAt.Add(2*time.Second),
		)
		if second.step.lifecycleTurnID != "turn-1" {
			t.Fatalf("steering step = %#v", second.step)
		}
		assertPositionedLifecycleTypes(t, second.entries, []transcript.EntryType{
			transcript.StepEndEntry,
			transcript.StepStartEntry,
		})
	})
}

func TestLifecycleCoordinatorDiscardsRetryAndOverflowSteps(t *testing.T) {
	for _, reason := range []string{"retry", "context_overflow"} {
		t.Run(reason, func(t *testing.T) {
			coordinator := &lifecycleCoordinator{}
			startedAt := lifecycleTestTime()
			coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
			first := coordinator.beginStepCheckpointWithID(1, "step-1", startedAt)
			coordinator.attachProviderRequestWithID("request-1")
			coordinator.commitStepCheckpoint("step-1", first.pendingCount)
			coordinator.completeStep(3, startedAt.Add(time.Second), "failed", "provider_request_failed")

			discarded := coordinator.discardLastStep(2, reason)
			if discarded.reason != reason || discarded.correlation.stepID != "step-1" ||
				discarded.correlation.requestID != "request-1" {
				t.Fatalf("discard decision = %#v", discarded)
			}
			second := coordinator.beginStepCheckpointWithID(2, "step-2", startedAt.Add(2*time.Second))
			if second.step.lifecycleTurnID != "turn-1" || second.step.stepID == discarded.correlation.stepID {
				t.Fatalf("replacement step = %#v after %#v", second.step, discarded)
			}
			for _, positioned := range second.entries {
				if positioned.messageIndex > 2 {
					t.Fatalf("entry %q remained after discarded message at %d", positioned.entry.Type, positioned.messageIndex)
				}
			}
		})
	}
}

func TestLifecycleCoordinatorHandlesCheckpointFailureAndCancellation(t *testing.T) {
	startedAt := lifecycleTestTime()
	t.Run("checkpoint failure never makes the step durable", func(t *testing.T) {
		coordinator := &lifecycleCoordinator{}
		coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
		coordinator.beginStepCheckpointWithID(1, "step-1", startedAt)
		coordinator.attachProviderRequestWithID("request-1")
		coordinator.completeStep(2, startedAt.Add(time.Second), "failed", "checkpoint_failed")
		checkpointErr := errors.New("checkpoint failed")
		terminal := coordinator.finishRun(2, startedAt.Add(2*time.Second), checkpointErr, checkpointErr)
		assertPositionedLifecycleTypes(t, terminal.entries, []transcript.EntryType{
			transcript.RunStartEntry,
			transcript.TurnStartEntry,
			transcript.TurnEndEntry,
			transcript.RunEndEntry,
		})
	})

	t.Run("cancellation closes every durable scope", func(t *testing.T) {
		coordinator := &lifecycleCoordinator{}
		coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
		checkpoint := coordinator.beginStepCheckpointWithID(1, "step-1", startedAt)
		coordinator.attachProviderRequestWithID("request-1")
		coordinator.commitStepCheckpoint("step-1", checkpoint.pendingCount)
		coordinator.completeStep(2, startedAt.Add(time.Second), "cancelled", "context_cancelled")
		terminal := coordinator.finishRun(2, startedAt.Add(2*time.Second), context.Canceled, nil)
		for _, positioned := range terminal.entries {
			if positioned.entry.Lifecycle.Status != transcript.LifecycleCancelled {
				t.Fatalf("cancelled boundary = %#v", positioned.entry)
			}
		}
		if terminal.status != transcript.LifecycleCancelled || terminal.reason != "context_cancelled" {
			t.Fatalf("cancelled terminal = %#v", terminal)
		}
	})

	t.Run("provider setup failure closes an open durable step", func(t *testing.T) {
		coordinator := &lifecycleCoordinator{}
		coordinator.startRunWithIDs(0, "run-1", "turn-1", startedAt)
		checkpoint := coordinator.beginStepCheckpointWithID(1, "step-1", startedAt)
		coordinator.attachProviderRequestWithID("request-1")
		coordinator.commitStepCheckpoint("step-1", checkpoint.pendingCount)
		terminal := coordinator.finishRun(
			1,
			startedAt.Add(time.Second),
			errors.New("provider setup failed"),
			nil,
		)
		assertPositionedLifecycleTypes(t, terminal.entries, []transcript.EntryType{
			transcript.StepEndEntry,
			transcript.TurnEndEntry,
			transcript.RunEndEntry,
		})
	})
}

func assertPositionedLifecycleTypes(
	t *testing.T,
	entries []positionedJournalEntry,
	want []transcript.EntryType,
) {
	t.Helper()
	got := make([]transcript.EntryType, len(entries))
	for index := range entries {
		got[index] = entries[index].entry.Type
	}
	if !slices.Equal(got, want) {
		t.Fatalf("lifecycle entries = %v, want %v", got, want)
	}
}

func lifecycleTestTime() time.Time {
	return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
}
