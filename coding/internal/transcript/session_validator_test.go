package transcript

import (
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
	if _, err := validator.ValidateAppend(invalidBatch); err == nil ||
		!strings.Contains(err.Error(), "repeats tool call id call-1") {
		t.Fatalf("ValidateAppend() error = %v", err)
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
	next, err := validator.ValidateAppend(validBatch)
	if err != nil {
		t.Fatalf("valid append after rejected candidate: %v", err)
	}
	if validator.NextSeq() != 3 || next.NextSeq() != 4 {
		t.Fatalf("validator cursors = %d -> %d, want 3 -> 4", validator.NextSeq(), next.NextSeq())
	}
}
