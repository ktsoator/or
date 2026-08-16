package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestSessionPersistsNestedLifecycle(t *testing.T) {
	store := &checkpointStore{}
	var checkpointEntries []transcript.Entry
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			checkpointEntries, _, _ = store.snapshot()
			return assistantEvents(model, "answer"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}

	if got := lifecycleTypes(checkpointEntries); !equalEntryTypes(got, []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
	}) {
		t.Fatalf("provider checkpoint lifecycle = %v", got)
	}

	entries, _, _ := store.snapshot()
	want := []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	}
	if got := lifecycleTypes(entries); !equalEntryTypes(got, want) {
		t.Fatalf("lifecycle types = %v, want %v", got, want)
	}
	assertLifecycleIDs(t, entries, 1, 1, 1)
	if repairs, err := transcript.RepairInterruptedLifecycle(entries); err != nil || len(repairs) != 0 {
		t.Fatalf("completed lifecycle repairs = %#v, %v", repairs, err)
	}
}

func TestSessionFollowUpStartsNewTurnBetweenMessages(t *testing.T) {
	store := &checkpointStore{}
	call := 0
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			return assistantEvents(model, "answer"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.FollowUp("more")
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if call != 2 {
		t.Fatalf("provider requests = %d, want 2", call)
	}

	entries, _, _ := store.snapshot()
	want := []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	}
	if got := lifecycleTypes(entries); !equalEntryTypes(got, want) {
		t.Fatalf("follow-up lifecycle = %v, want %v", got, want)
	}
	assertLifecycleIDs(t, entries, 1, 2, 2)

	firstAssistant := messageEntryIndex(entries, 0, false)
	followUp := messageEntryIndex(entries, 1, true)
	firstStepEnd := entryTypeIndexAfter(entries, transcript.StepEndEntry, firstAssistant)
	firstTurnEnd := entryTypeIndexAfter(entries, transcript.TurnEndEntry, firstStepEnd)
	secondTurnStart := entryTypeIndexAfter(entries, transcript.TurnStartEntry, firstTurnEnd)
	secondStepStart := entryTypeIndexAfter(entries, transcript.StepStartEntry, followUp)
	if !(firstAssistant < firstStepEnd && firstStepEnd < firstTurnEnd &&
		firstTurnEnd < secondTurnStart && secondTurnStart < followUp &&
		followUp < secondStepStart) {
		t.Fatalf(
			"follow-up boundary positions assistant=%d stepEnd=%d turnEnd=%d turnStart=%d followUp=%d stepStart=%d",
			firstAssistant,
			firstStepEnd,
			firstTurnEnd,
			secondTurnStart,
			followUp,
			secondStepStart,
		)
	}
}

func TestSessionSteeringStaysInCurrentTurn(t *testing.T) {
	store := &checkpointStore{}
	call := 0
	var session *Session
	var err error
	session, err = New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			if call == 1 {
				session.Steer("adjust")
			}
			return assistantEvents(model, "answer"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if call != 2 {
		t.Fatalf("provider requests = %d, want 2", call)
	}

	entries, _, _ := store.snapshot()
	want := []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.StepStartEntry,
		transcript.StepEndEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	}
	if got := lifecycleTypes(entries); !equalEntryTypes(got, want) {
		t.Fatalf("steering lifecycle = %v, want %v", got, want)
	}
	assertLifecycleIDs(t, entries, 1, 1, 2)

	firstAssistant := messageEntryIndex(entries, 0, false)
	steering := messageEntryIndex(entries, 1, true)
	firstStepEnd := entryTypeIndexAfter(entries, transcript.StepEndEntry, firstAssistant)
	secondStepStart := entryTypeIndexAfter(entries, transcript.StepStartEntry, steering)
	if !(firstAssistant < firstStepEnd && firstStepEnd < steering && steering < secondStepStart) {
		t.Fatalf(
			"steering positions assistant=%d stepEnd=%d steering=%d stepStart=%d",
			firstAssistant,
			firstStepEnd,
			steering,
			secondStepStart,
		)
	}
}

func TestSessionToolLoopKeepsStepsInOneTurn(t *testing.T) {
	store := &checkpointStore{}
	call := 0
	tool := tools.Tool{
		AgentTool: agent.AgentTool{
			Definition: llm.MustTool[checkpointToolArgs]("echo", "echo text"),
			Execute: func(
				context.Context,
				string,
				json.RawMessage,
				func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				return agent.ToolResult{}, nil
			},
		},
		AccessFor: tools.InternalAccess,
	}
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{tool},
		Store: store,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			call++
			if call == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{&llm.ToolCall{
					ID: "call-1", Name: "echo", Arguments: map[string]any{"text": "one"},
				}}
				return finalEvents(llm.EventDone, &message), nil
			}
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "use a tool"); err != nil {
		t.Fatal(err)
	}
	entries, _, _ := store.snapshot()
	assertLifecycleIDs(t, entries, 1, 1, 2)
}

func TestProviderCheckpointFailureDoesNotPersistStepStart(t *testing.T) {
	storeErr := errors.New("checkpoint unavailable")
	store := &checkpointStore{failErr: storeErr, failOnce: true}
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		StreamFn: func(
			context.Context,
			llm.Model,
			llm.Context,
			llm.StreamOptions,
		) (<-chan llm.Event, error) {
			return nil, errors.New("provider must not be called")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "question"); !errors.Is(err, storeErr) {
		t.Fatalf("Prompt() error = %v, want checkpoint error", err)
	}

	entries, _, _ := store.snapshot()
	for _, entry := range entries {
		if entry.Type == transcript.StepStartEntry || entry.Type == transcript.StepEndEntry {
			t.Fatalf("failed provider checkpoint persisted step boundary: %#v", entry)
		}
	}
	want := []transcript.EntryType{
		transcript.RunStartEntry,
		transcript.TurnStartEntry,
		transcript.TurnEndEntry,
		transcript.RunEndEntry,
	}
	if got := lifecycleTypes(entries); !equalEntryTypes(got, want) {
		t.Fatalf("failed checkpoint lifecycle = %v, want %v", got, want)
	}
}

func lifecycleTypes(entries []transcript.Entry) []transcript.EntryType {
	var result []transcript.EntryType
	for _, entry := range entries {
		if entry.Lifecycle != nil {
			result = append(result, entry.Type)
		}
	}
	return result
}

func assertLifecycleIDs(
	t *testing.T,
	entries []transcript.Entry,
	wantRuns, wantTurns, wantSteps int,
) {
	t.Helper()
	runs := map[string]bool{}
	turns := map[string]string{}
	steps := map[string]string{}
	for _, entry := range entries {
		if entry.Lifecycle == nil {
			continue
		}
		lifecycle := entry.Lifecycle
		runs[lifecycle.RunID] = true
		if lifecycle.TurnID != "" {
			if previous := turns[lifecycle.TurnID]; previous != "" && previous != lifecycle.RunID {
				t.Fatalf("turn %s changed run from %s to %s", lifecycle.TurnID, previous, lifecycle.RunID)
			}
			turns[lifecycle.TurnID] = lifecycle.RunID
		}
		if lifecycle.StepID != "" {
			if previous := steps[lifecycle.StepID]; previous != "" && previous != lifecycle.TurnID {
				t.Fatalf("step %s changed turn from %s to %s", lifecycle.StepID, previous, lifecycle.TurnID)
			}
			steps[lifecycle.StepID] = lifecycle.TurnID
		}
	}
	if len(runs) != wantRuns || len(turns) != wantTurns || len(steps) != wantSteps {
		t.Fatalf("lifecycle id counts = runs %d, turns %d, steps %d", len(runs), len(turns), len(steps))
	}
}

func equalEntryTypes(left, right []transcript.EntryType) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func messageEntryIndex(entries []transcript.Entry, ordinal int, user bool) int {
	seen := 0
	for index, entry := range entries {
		message := llmEntry(entry)
		if message == nil {
			continue
		}
		_, isUser := message.(*llm.UserMessage)
		if isUser != user {
			continue
		}
		if seen == ordinal {
			return index
		}
		seen++
	}
	return -1
}

func entryTypeIndexAfter(entries []transcript.Entry, entryType transcript.EntryType, after int) int {
	for index := after + 1; index < len(entries); index++ {
		if entries[index].Type == entryType {
			return index
		}
	}
	return -1
}
