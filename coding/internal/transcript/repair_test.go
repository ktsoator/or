package transcript

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestRecoverSessionClassifiesInterruptedToolsInModelOrder(t *testing.T) {
	assistant := repairAssistant(
		llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{"path": "one"}},
		llm.ToolCall{ID: "call-2", Name: "write", Arguments: map[string]any{"path": "two"}},
	)
	entries := append(repairStepPrefix(true),
		NewMessage(agent.FromLLM(assistant)),
		NewToolCall(ToolCall{
			ToolCallID: "call-2", ToolName: "write", Arguments: []byte(`{"path":"two"}`),
		}),
	)
	entries = sequencedForTest(entries...)

	_, repairs, err := RecoverSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(repairs) != 7 {
		t.Fatalf("repair entries = %d, want two result/outcome pairs and three boundaries", len(repairs))
	}
	assertRepairPair(t, repairs[0:2], "call-1", ToolNotStarted, "did not start")
	assertRepairPair(t, repairs[2:4], "call-2", ToolOutcomeUnknown, "outcome is unknown")

	projected, err := BuildContext(append(entries, repairs...))
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 4 {
		t.Fatalf("repaired context messages = %d, want user, assistant, and two results", len(projected))
	}
	repaired := append(entries, repairs...)
	if _, more, err := RecoverSession(repaired); err != nil || len(more) != 0 {
		t.Fatalf("second repair = %#v, %v, want no-op", more, err)
	}
}

func TestRecoverSessionAcceptsBlockedResultWithoutIntent(t *testing.T) {
	assistant := repairAssistant(llm.ToolCall{ID: "call-1", Name: "write", Arguments: map[string]any{}})
	entries := append(repairStepPrefix(true),
		NewMessage(agent.FromLLM(assistant)),
		NewMessage(agent.FromLLM(&llm.ToolResultMessage{
			ToolCallID: "call-1", ToolName: "write", IsError: true,
			Content: []llm.ToolResultContent{&llm.TextContent{Text: "permission denied"}},
		})),
	)
	entries = sequencedForTest(entries...)

	_, repairs, err := RecoverSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryTypes(repairs); !equalTypes(got, []EntryType{
		StepEndEntry, TurnEndEntry, RunEndEntry,
	}) {
		t.Fatalf("repairs = %v, want only lifecycle closure", got)
	}
}

func TestRecoverSessionAllowsContextAttachmentBeforeResult(t *testing.T) {
	entries := append(repairStepPrefix(false),
		NewMessage(agent.FromLLM(repairAssistant(
			llm.ToolCall{ID: "call-1", Name: "Skill", Arguments: map[string]any{"name": "review"}},
		))),
		NewToolCall(ToolCall{
			ToolCallID: "call-1", ToolName: "Skill", Arguments: []byte(`{"name":"review"}`),
		}),
		NewContext(ContextAttachment{
			AttachmentID: "activated-review",
			Epoch:        1,
			Kind:         "activated_skill",
			Placement:    "before_latest_user",
			Revision:     "revision",
			Rendered:     "review instructions",
		}),
	)
	entries = sequencedForTest(entries...)

	_, repairs, err := RecoverSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(repairs) != 5 {
		t.Fatalf("repairs = %#v, want one result/outcome pair and three boundaries", repairs)
	}
	assertRepairPair(t, repairs[:2], "call-1", ToolOutcomeUnknown, "outcome is unknown")
}

func TestRecoverSessionRejectsInvalidToolHistory(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		want    string
	}{
		{
			name: "orphan intent",
			entries: append(repairStepPrefix(false), NewToolCall(ToolCall{
				ToolCallID: "call-1", ToolName: "read", Arguments: []byte(`{}`),
			})),
			want: "no unresolved assistant call",
		},
		{
			name: "out of order result",
			entries: append(repairStepPrefix(false),
				NewMessage(agent.FromLLM(repairAssistant(
					llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{}},
					llm.ToolCall{ID: "call-2", Name: "read", Arguments: map[string]any{}},
				))),
				NewMessage(agent.FromLLM(&llm.ToolResultMessage{ToolCallID: "call-2", ToolName: "read"})),
			),
			want: "out of model order",
		},
		{
			name: "new user before result",
			entries: append(repairStepPrefix(false),
				NewMessage(agent.FromLLM(repairAssistant(
					llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{}},
				))),
				NewMessage(agent.UserMessage("continue")),
			),
			want: "follows unresolved tool call",
		},
		{
			name: "dispatch arguments differ",
			entries: append(repairStepPrefix(false),
				NewMessage(agent.FromLLM(repairAssistant(
					llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{"path": "one"}},
				))),
				NewToolCall(ToolCall{
					ToolCallID: "call-1", ToolName: "read", Arguments: []byte(`{"path":"two"}`),
				}),
			),
			want: "arguments differ",
		},
		{
			name: "outcome before result",
			entries: append(repairStepPrefix(false),
				NewMessage(agent.FromLLM(repairAssistant(
					llm.ToolCall{ID: "call-1", Name: "read", Arguments: map[string]any{}},
				))),
				NewToolOutcome(ToolOutcome{
					ToolCallID: "call-1", Status: agent.ToolOutcomeSuccess,
				}),
			),
			want: "precedes result",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := RecoverSession(sequencedForTest(test.entries...))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RecoverSession() error = %v, want %q", err, test.want)
			}
		})
	}
}

func entryTypes(entries []Entry) []EntryType {
	result := make([]EntryType, len(entries))
	for index, entry := range entries {
		result[index] = entry.Type
	}
	return result
}

func equalTypes(left, right []EntryType) bool {
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

func repairStepPrefix(includeUser bool) []Entry {
	entries := []Entry{
		NewRunStart("repair-run"),
		NewTurnStart("repair-run", "repair-turn"),
	}
	if includeUser {
		entries = append(entries, NewMessage(agent.UserMessage("work")))
	}
	return append(entries, NewStepStart("repair-run", "repair-turn", "repair-step"))
}

func repairAssistant(calls ...llm.ToolCall) *llm.AssistantMessage {
	content := make([]llm.AssistantContent, len(calls))
	for index := range calls {
		call := calls[index]
		content[index] = &call
	}
	return &llm.AssistantMessage{Content: content, StopReason: llm.StopReasonToolUse}
}

func assertRepairPair(t *testing.T, entries []Entry, callID, code, text string) {
	t.Helper()
	if len(entries) != 2 {
		t.Fatalf("repair pair = %#v", entries)
	}
	message, ok := agent.ToLLM(entries[0].Message)
	if entries[0].Type != MessageEntry || !ok {
		t.Fatalf("repair message entry = %#v", entries[0])
	}
	result, ok := message.(*llm.ToolResultMessage)
	if !ok || result.ToolCallID != callID || !result.IsError ||
		len(result.Content) != 1 || !strings.Contains(result.Content[0].(*llm.TextContent).Text, text) {
		t.Fatalf("repair result = %#v", result)
	}
	outcome := entries[1]
	if outcome.Type != ToolOutcomeEntry || outcome.ToolOutcome == nil ||
		outcome.ToolOutcome.ToolCallID != callID ||
		outcome.ToolOutcome.Status != agent.ToolOutcomeFailed ||
		outcome.ToolOutcome.ErrorCode != code {
		t.Fatalf("repair outcome = %#v", outcome)
	}
}
