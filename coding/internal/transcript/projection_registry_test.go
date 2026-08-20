package transcript

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type entryTypeProjection struct {
	types []EntryType
}

func (*entryTypeProjection) ProjectionKey() string { return "test/entry-types" }

func (p *entryTypeProjection) ApplyProjection(event ProjectionEvent) {
	p.types = append(p.types, event.Entry.Type)
}

func (p *entryTypeProjection) SnapshotProjection() (any, error) {
	return append([]EntryType(nil), p.types...), nil
}

type eventCaptureProjection struct {
	events []ProjectionEvent
}

func (*eventCaptureProjection) ProjectionKey() string { return "test/events" }

func (p *eventCaptureProjection) ApplyProjection(event ProjectionEvent) {
	p.events = append(p.events, event)
}

func (p *eventCaptureProjection) SnapshotProjection() (any, error) {
	return append([]ProjectionEvent(nil), p.events...), nil
}

type snapshotCountingProjection struct {
	snapshots int
}

func (*snapshotCountingProjection) ProjectionKey() string { return "test/snapshot-count" }

func (*snapshotCountingProjection) ApplyProjection(ProjectionEvent) {}

func (p *snapshotCountingProjection) SnapshotProjection() (any, error) {
	p.snapshots++
	return p.snapshots, nil
}

func TestProjectionRegistryDrivesRegisteredUnitsAtOneWatermark(t *testing.T) {
	registry := NewProjectionRegistry()
	types := &entryTypeProjection{}
	session := NewSessionProjectionUnit()
	if err := registry.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(session); err != nil {
		t.Fatal(err)
	}

	entries := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewMessage(agent.UserMessage("question")),
	)
	validator, err := validateSession(entries[:2], registry)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := validator.PrepareAppend(entries[2:])
	if err != nil {
		t.Fatal(err)
	}

	before, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	viewBefore := before.Values[SessionProjectionKey].(*SessionProjection)
	if before.AsOfSeq != 1 || viewBefore.AsOfSeq != 1 || len(viewBefore.Messages) != 0 {
		t.Fatalf("projection advanced before commit: %#v", before)
	}

	batch.Commit()
	after, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	viewAfter := after.Values[SessionProjectionKey].(*SessionProjection)
	if after.AsOfSeq != 2 || viewAfter.AsOfSeq != 2 || len(viewAfter.Messages) != 1 {
		t.Fatalf("projection after commit = %#v", after)
	}
	if got := after.Values[types.ProjectionKey()].([]EntryType); !reflect.DeepEqual(got, []EntryType{
		RunStartEntry, TurnStartEntry, MessageEntry,
	}) {
		t.Fatalf("entry type projection = %v", got)
	}
}

func TestProjectionRegistrySnapshotsOneKeyWithoutCloningOtherUnits(t *testing.T) {
	registry := NewProjectionRegistry()
	types := &entryTypeProjection{}
	other := &snapshotCountingProjection{}
	if err := registry.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(other); err != nil {
		t.Fatal(err)
	}
	entries := sequencedForTest(NewRunStart("run-1"))
	if _, err := validateSession(entries, registry); err != nil {
		t.Fatal(err)
	}

	snapshot, err := registry.SnapshotKey(types.ProjectionKey())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AsOfSeq != 0 || other.snapshots != 0 {
		t.Fatalf("targeted snapshot = %#v, unrelated snapshots = %d", snapshot, other.snapshots)
	}
	if got := snapshot.Values[types.ProjectionKey()].([]EntryType); !reflect.DeepEqual(
		got,
		[]EntryType{RunStartEntry},
	) {
		t.Fatalf("targeted projection = %v", got)
	}
	if _, err := registry.SnapshotKey("missing"); err == nil ||
		!strings.Contains(err.Error(), "not registered") {
		t.Fatalf("missing projection error = %v", err)
	}
}

func TestIncrementalSessionProjectionMatchesCompleteReplay(t *testing.T) {
	entries := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewMessage(agent.UserMessage("question")),
		NewStepStart("run-1", "turn-1", "step-1"),
		NewMessage(agent.FromLLM(&llm.AssistantMessage{
			Content:    []llm.AssistantContent{&llm.TextContent{Text: "answer"}},
			StopReason: llm.StopReasonStop,
		})),
		NewStepEnd("run-1", "turn-1", "step-1", LifecycleCompleted, ""),
		NewTurnEnd("run-1", "turn-1", LifecycleCompleted, ""),
		NewRunEnd("run-1", LifecycleCompleted, ""),
	)
	registry := NewProjectionRegistry()
	unit := NewSessionProjectionUnit()
	if err := registry.Register(unit); err != nil {
		t.Fatal(err)
	}
	validator, err := validateSession(entries[:2], registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, bounds := range [][2]int{{2, 5}, {5, 8}} {
		prepared, err := validator.PrepareAppend(entries[bounds[0]:bounds[1]])
		if err != nil {
			t.Fatal(err)
		}
		prepared.Commit()
	}

	incremental, err := unit.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ProjectSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(incremental, replayed) {
		t.Fatalf("incremental projection differs from replay\nincremental: %#v\nreplayed: %#v", incremental, replayed)
	}
}

func TestSessionProjectionSnapshotsAreDetached(t *testing.T) {
	registry := NewProjectionRegistry()
	unit := NewSessionProjectionUnit()
	if err := registry.Register(unit); err != nil {
		t.Fatal(err)
	}
	entries := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewMessage(agent.UserMessage("original")),
	)
	if _, err := validateSession(entries, registry); err != nil {
		t.Fatal(err)
	}

	first, err := unit.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	message, _ := agent.ToLLM(first.Messages[0].Message)
	message.(*llm.UserMessage).Content[0].(*llm.TextContent).Text = "mutated"
	first.Runs[0].Turns[0].ID = "mutated"

	second, err := unit.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	message, _ = agent.ToLLM(second.Messages[0].Message)
	if got := message.(*llm.UserMessage).Content[0].(*llm.TextContent).Text; got != "original" {
		t.Fatalf("live projection message changed through snapshot: %q", got)
	}
	if second.Runs[0].Turns[0].ID != "turn-1" {
		t.Fatalf("live projection lifecycle changed through snapshot: %#v", second.Runs)
	}
}

func TestProjectionEventsDetachMutableEntryPayloads(t *testing.T) {
	exitCode := 17
	entries := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewStepStart("run-1", "turn-1", "step-1"),
		NewMessage(agent.FromLLM(&llm.AssistantMessage{
			Content: []llm.AssistantContent{&llm.ToolCall{
				ID: "call-1", Name: "read", Arguments: map[string]any{"path": "one"},
			}},
			StopReason: llm.StopReasonToolUse,
		})),
		NewToolCall(ToolCall{
			ToolCallID: "call-1", ToolName: "read", Arguments: []byte(`{"path":"one"}`),
		}),
		NewMessage(agent.FromLLM(&llm.ToolResultMessage{
			ToolCallID: "call-1", ToolName: "read",
			Content: []llm.ToolResultContent{&llm.TextContent{Text: "done"}},
		})),
		NewToolOutcome(ToolOutcome{
			ToolCallID: "call-1", Status: agent.ToolOutcomeSuccess,
			ExitCode: &exitCode, Data: []byte(`{"path":"one"}`),
		}),
	)
	registry := NewProjectionRegistry()
	capture := &eventCaptureProjection{}
	if err := registry.Register(capture); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSession(entries, registry); err != nil {
		t.Fatal(err)
	}

	assistant, _ := agent.ToLLM(entries[3].Message)
	assistant.(*llm.AssistantMessage).ToolCalls()[0].Arguments["path"] = "mutated"
	entries[4].ToolCall.Arguments[9] = 'x'
	entries[6].ToolOutcome.Data[9] = 'x'
	*entries[6].ToolOutcome.ExitCode = 99

	projectedAssistant, _ := agent.ToLLM(capture.events[3].Entry.Message)
	if got := projectedAssistant.(*llm.AssistantMessage).ToolCalls()[0].Arguments["path"]; got != "one" {
		t.Fatalf("projected assistant arguments changed through source: %v", got)
	}
	if got := string(capture.events[4].Entry.ToolCall.Arguments); got != `{"path":"one"}` {
		t.Fatalf("projected dispatch arguments changed through source: %s", got)
	}
	outcome := capture.events[6].Entry.ToolOutcome
	if got := string(outcome.Data); got != `{"path":"one"}` || outcome.ExitCode == nil || *outcome.ExitCode != 17 {
		t.Fatalf("projected outcome changed through source: %#v", outcome)
	}
}

func TestProjectionRegistryRejectsDuplicateAndLateRegistration(t *testing.T) {
	registry := NewProjectionRegistry()
	if err := registry.Register(&entryTypeProjection{}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&entryTypeProjection{}); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate registration error = %v", err)
	}
	entries := sequencedForTest(NewRunStart("run-1"))
	if _, err := validateSession(entries, registry); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(NewSessionProjectionUnit()); err == nil ||
		!strings.Contains(err.Error(), "after replay began") {
		t.Fatalf("late registration error = %v", err)
	}
}
