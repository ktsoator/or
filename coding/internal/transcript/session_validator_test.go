package transcript

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestSessionValidatorRejectsCandidateWithoutMutatingPrefix(t *testing.T) {
	prefix := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewStepStart("run-1", "turn-1", "step-1"),
	)
	validator, err := ValidateSession(prefix)
	if err != nil {
		t.Fatal(err)
	}

	invalid := NewMessage(agent.FromLLM(&llm.AssistantMessage{
		Content: []llm.AssistantContent{
			&llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{}},
			&llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{}},
		},
		StopReason: llm.StopReasonToolUse,
	}))
	invalidBatch, err := SequenceEntries([]Entry{invalid}, validator.NextSeq())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.PrepareAppend(invalidBatch); err == nil ||
		!strings.Contains(err.Error(), "repeats tool call id call-1") {
		t.Fatalf("PrepareAppend() error = %v", err)
	}

	valid := NewMessage(agent.FromLLM(&llm.AssistantMessage{
		Content: []llm.AssistantContent{
			&llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{}},
		},
		StopReason: llm.StopReasonToolUse,
	}))
	validBatch, err := SequenceEntries([]Entry{valid}, validator.NextSeq())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := validator.PrepareAppend(validBatch)
	if err != nil {
		t.Fatalf("valid append after rejected candidate: %v", err)
	}
	if validator.NextSeq() != 3 {
		t.Fatalf("validator cursor before commit = %d, want 3", validator.NextSeq())
	}
	prepared.Commit()
	if validator.NextSeq() != 4 {
		t.Fatalf("validator cursor after commit = %d, want 4", validator.NextSeq())
	}
}

func TestPreparedAppendKeepsTouchedToolStatePrivateUntilCommit(t *testing.T) {
	prefix := sequencedForTest(
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewStepStart("run-1", "turn-1", "step-1"),
		NewMessage(agent.FromLLM(&llm.AssistantMessage{
			Content: []llm.AssistantContent{&llm.ToolCall{
				ID: "call-1", Name: "read", Arguments: map[string]any{"path": "one"},
			}},
			StopReason: llm.StopReasonToolUse,
		})),
	)
	validator, err := ValidateSession(prefix)
	if err != nil {
		t.Fatal(err)
	}

	wrong := NewToolCall(ToolCall{
		ToolCallID: "call-1", ToolName: "read", Arguments: []byte(`{"path":"two"}`),
	})
	wrongBatch, err := SequenceEntries([]Entry{wrong}, validator.NextSeq())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.PrepareAppend(wrongBatch); err == nil ||
		!strings.Contains(err.Error(), "arguments differ") {
		t.Fatalf("PrepareAppend() error = %v, want argument mismatch", err)
	}
	if validator.reducer.tools["call-1"].DispatchEntryID != "" {
		t.Fatal("rejected dispatch changed committed tool state")
	}

	correct := NewToolCall(ToolCall{
		ToolCallID: "call-1", ToolName: "read", Arguments: []byte(`{"path":"one"}`),
	})
	correctBatch, err := SequenceEntries([]Entry{correct}, validator.NextSeq())
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := validator.PrepareAppend(correctBatch)
	if err != nil {
		t.Fatal(err)
	}
	if validator.reducer.tools["call-1"].DispatchEntryID != "" {
		t.Fatal("prepared dispatch changed committed tool state")
	}
	second, err := validator.PrepareAppend(correctBatch)
	if err != nil {
		t.Fatalf("same append could not be prepared again before commit: %v", err)
	}
	second.Commit()
	if got := validator.reducer.tools["call-1"].DispatchEntryID; got != correct.ID {
		t.Fatalf("committed dispatch entry = %q, want %q", got, correct.ID)
	}
	requirePanicContains(t, "stale", discarded.Commit)
	requirePanicContains(t, "invalid prepared append", second.Commit)
}

func TestPreparedAppendStagesOnlyBatchDelta(t *testing.T) {
	const runs = 100
	prefix := make([]Entry, 0, 2*runs)
	for index := 0; index < runs; index++ {
		runID := fmt.Sprintf("run-%d", index)
		prefix = append(prefix,
			NewRunStart(runID),
			NewRunEnd(runID, LifecycleCompleted, ""),
		)
	}
	prefix = sequencedForTest(prefix...)
	validator, err := ValidateSession(prefix)
	if err != nil {
		t.Fatal(err)
	}

	next := NewRunStart("run-next")
	batch, err := SequenceEntries([]Entry{next}, validator.NextSeq())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := validator.PrepareAppend(batch)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.stage.parent != validator.reducer ||
		len(prepared.stage.entryIDs) != 1 ||
		len(prepared.stage.runIDs) != 1 ||
		len(prepared.stage.turnIDs) != 0 ||
		len(prepared.stage.tools) != 0 {
		t.Fatalf("prepared reducer contains more than the batch delta: %#v", prepared.stage)
	}
	if len(validator.reducer.entryIDs) != 2*runs {
		t.Fatalf("committed entry ids = %d, want %d", len(validator.reducer.entryIDs), 2*runs)
	}
	prepared.Commit()
	if len(validator.reducer.entryIDs) != 2*runs+1 || validator.NextSeq() != 2*runs+1 {
		t.Fatalf(
			"committed state = %d ids at seq %d, want %d",
			len(validator.reducer.entryIDs), validator.NextSeq(), 2*runs+1,
		)
	}
}

func requirePanicContains(t *testing.T, want string, call func()) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil || !strings.Contains(fmt.Sprint(value), want) {
			t.Fatalf("panic = %v, want text %q", value, want)
		}
	}()
	call()
}
