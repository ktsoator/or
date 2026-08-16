package transcript

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestProjectSessionBuildsLifecycleAndToolAssociations(t *testing.T) {
	contextEntry := NewContext(ContextAttachment{
		AttachmentID: "base:1", Epoch: 1, Kind: "base", Placement: "prefix",
		Revision: "revision-1", Rendered: "runtime context",
	})
	runStart := NewRunStart("run-1")
	turnStart := NewTurnStart("run-1", "turn-1")
	user := NewMessage(agent.UserMessage("write the release note"))
	stepOneStart := NewStepStart("run-1", "turn-1", "step-1")
	assistantTool := NewMessage(agent.FromLLM(&llm.AssistantMessage{
		Content: []llm.AssistantContent{
			&llm.TextContent{Text: "I will write it."},
			&llm.ToolCall{
				ID: "call-1", Name: "write",
				Arguments: map[string]any{"path": "RELEASE.md"},
			},
		},
		StopReason: llm.StopReasonToolUse,
	}))
	dispatch := NewToolCall(ToolCall{
		ToolCallID: "call-1", ToolName: "write",
		Arguments: json.RawMessage(`{"path":"RELEASE.md"}`),
	})
	result := NewMessage(agent.FromLLM(&llm.ToolResultMessage{
		ToolCallID: "call-1", ToolName: "write",
		Content: []llm.ToolResultContent{&llm.TextContent{Text: "created"}},
	}))
	outcome := NewToolOutcome(ToolOutcome{
		ToolCallID: "call-1", Status: agent.ToolOutcomeSuccess,
		DataKind: "file", Data: json.RawMessage(`{"path":"RELEASE.md"}`),
	})
	stepOneEnd := NewStepEnd("run-1", "turn-1", "step-1", LifecycleCompleted, "")
	stepTwoStart := NewStepStart("run-1", "turn-1", "step-2")
	assistantFinal := NewMessage(agent.FromLLM(&llm.AssistantMessage{
		Content:    []llm.AssistantContent{&llm.TextContent{Text: "Done."}},
		StopReason: llm.StopReasonStop,
	}))
	stepTwoEnd := NewStepEnd("run-1", "turn-1", "step-2", LifecycleCompleted, "")
	turnEnd := NewTurnEnd("run-1", "turn-1", LifecycleCompleted, "")
	runEnd := NewRunEnd("run-1", LifecycleCompleted, "")

	entries := []Entry{
		runStart, turnStart, user,
		stepOneStart, contextEntry, assistantTool, dispatch, result, outcome, stepOneEnd,
		stepTwoStart, assistantFinal, stepTwoEnd,
		turnEnd, runEnd,
	}
	stampProjectionEntries(entries)

	projection, err := ProjectSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if projection.AppliedEntries != len(entries) || projection.Open != (ProjectedLifecycle{}) {
		t.Fatalf("projection boundary = entries %d open %#v", projection.AppliedEntries, projection.Open)
	}
	replayed, err := ProjectSession(entries)
	if err != nil || !reflect.DeepEqual(projection, replayed) {
		t.Fatalf("deterministic replay = %#v, %v", replayed, err)
	}
	if len(projection.Runs) != 1 || len(projection.Runs[0].Turns) != 1 ||
		len(projection.Runs[0].Turns[0].Steps) != 2 {
		t.Fatalf("lifecycle projection = %#v", projection.Runs)
	}
	run := projection.Runs[0]
	turn := run.Turns[0]
	if run.ID != "run-1" || run.Status != LifecycleCompleted ||
		turn.ID != "turn-1" || turn.Status != LifecycleCompleted {
		t.Fatalf("terminal lifecycle = run %#v turn %#v", run, turn)
	}
	if run.StartedAt != entries[0].Timestamp || run.CompletedAt != entries[len(entries)-1].Timestamp {
		t.Fatalf("run timing = %v..%v", run.StartedAt, run.CompletedAt)
	}
	if turn.Steps[0].ID != "step-1" || turn.Steps[1].ID != "step-2" ||
		turn.Steps[1].Status != LifecycleCompleted {
		t.Fatalf("steps = %#v", turn.Steps)
	}

	if len(projection.Messages) != 4 {
		t.Fatalf("messages = %#v", projection.Messages)
	}
	if projection.Messages[0].RunID != "run-1" || projection.Messages[0].TurnID != "turn-1" ||
		projection.Messages[0].StepID != "" {
		t.Fatalf("user ownership = %#v", projection.Messages[0])
	}
	if projection.Messages[1].StepID != "step-1" ||
		projection.Messages[2].StepID != "step-1" ||
		projection.Messages[3].StepID != "step-2" {
		t.Fatalf("step message ownership = %#v", projection.Messages)
	}
	sourceUser, _ := agent.ToLLM(entries[2].Message)
	sourceUser.(*llm.UserMessage).Content[0].(*llm.TextContent).Text = "mutated"
	projectedUser, _ := agent.ToLLM(projection.Messages[0].Message)
	if got := projectedUser.(*llm.UserMessage).Content[0].(*llm.TextContent).Text; got != "write the release note" {
		t.Fatalf("projected message changed with source: %q", got)
	}

	if len(projection.ToolCalls) != 1 {
		t.Fatalf("tools = %#v", projection.ToolCalls)
	}
	tool := projection.ToolCalls[0]
	if tool.ToolCallID != "call-1" || tool.ToolName != "write" ||
		tool.RunID != "run-1" || tool.TurnID != "turn-1" || tool.StepID != "step-1" ||
		tool.AssistantMessageEntryID != assistantTool.ID ||
		tool.DispatchEntryID != dispatch.ID ||
		tool.ResultMessageEntryID != result.ID ||
		tool.OutcomeEntryID != outcome.ID ||
		tool.Outcome == nil || tool.Outcome.Status != agent.ToolOutcomeSuccess {
		t.Fatalf("tool projection = %#v", tool)
	}
	if len(projection.Contexts) != 1 || projection.Contexts[0].RunID != "run-1" ||
		projection.Contexts[0].TurnID != "turn-1" || projection.Contexts[0].StepID != "step-1" ||
		projection.Contexts[0].Attachment.AttachmentID != "base:1" {
		t.Fatalf("contexts = %#v", projection.Contexts)
	}
}

func TestProjectSessionPreservesOpenFollowUpTail(t *testing.T) {
	entries := []Entry{
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewMessage(agent.UserMessage("first")),
		NewStepStart("run-1", "turn-1", "step-1"),
		NewMessage(agent.FromLLM(&llm.AssistantMessage{
			Content:    []llm.AssistantContent{&llm.TextContent{Text: "first answer"}},
			StopReason: llm.StopReasonStop,
		})),
		NewStepEnd("run-1", "turn-1", "step-1", LifecycleCompleted, ""),
		NewTurnEnd("run-1", "turn-1", LifecycleCompleted, ""),
		NewTurnStart("run-1", "turn-2"),
		NewMessage(agent.UserMessage("follow up")),
		NewStepStart("run-1", "turn-2", "step-2"),
		NewMessage(agent.FromLLM(&llm.AssistantMessage{
			Content: []llm.AssistantContent{&llm.ToolCall{
				ID: "call-open", Name: "read", Arguments: map[string]any{"path": "README.md"},
			}},
			StopReason: llm.StopReasonToolUse,
		})),
		NewToolCall(ToolCall{
			ToolCallID: "call-open", ToolName: "read",
			Arguments: json.RawMessage(`{"path":"README.md"}`),
		}),
	}
	stampProjectionEntries(entries)

	projection, err := ProjectSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Open != (ProjectedLifecycle{
		RunID: "run-1", TurnID: "turn-2", StepID: "step-2",
	}) {
		t.Fatalf("open lifecycle = %#v", projection.Open)
	}
	if len(projection.Runs) != 1 || len(projection.Runs[0].Turns) != 2 {
		t.Fatalf("turns = %#v", projection.Runs)
	}
	if projection.Runs[0].Turns[0].Status != LifecycleCompleted ||
		projection.Runs[0].Turns[1].Status != "" {
		t.Fatalf("turn status = %#v", projection.Runs[0].Turns)
	}
	followUp := projection.Messages[2]
	if followUp.TurnID != "turn-2" || followUp.StepID != "" {
		t.Fatalf("follow-up ownership = %#v", followUp)
	}
	tool := projection.ToolCalls[0]
	if tool.DispatchEntryID == "" || tool.ResultMessageEntryID != "" || tool.Outcome != nil {
		t.Fatalf("open tool = %#v", tool)
	}
}

func TestProjectSessionRejectsMessagesWithoutLifecycle(t *testing.T) {
	user := NewMessage(agent.UserMessage("question"))
	entries := []Entry{user}
	stampProjectionEntries(entries)

	_, err := ProjectSession(entries)
	if err == nil || !strings.Contains(err.Error(), "has no open turn") {
		t.Fatalf("ProjectSession() error = %v, want strict lifecycle rejection", err)
	}
}

func TestProjectSessionAssociatesAndDetachesCompaction(t *testing.T) {
	runStart := NewRunStart("run-1")
	turnStart := NewTurnStart("run-1", "turn-1")
	user := NewMessage(agent.UserMessage("question"))
	compaction := NewCompaction(Compaction{
		Summary: "earlier work", FirstKeptEntryID: user.ID,
		ReadFiles: []string{"README.md"}, ModifiedFiles: []string{"main.go"},
	})
	entries := []Entry{runStart, turnStart, user, compaction}
	stampProjectionEntries(entries)

	projection, err := ProjectSession(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Compactions) != 1 {
		t.Fatalf("compactions = %#v", projection.Compactions)
	}
	projected := projection.Compactions[0]
	if projected.RunID != "run-1" || projected.TurnID != "turn-1" ||
		projected.StepID != "" || projected.Compaction.FirstKeptEntryID != user.ID {
		t.Fatalf("compaction ownership = %#v", projected)
	}
	entries[3].Compaction.ReadFiles[0] = "mutated"
	if projected.Compaction.ReadFiles[0] != "README.md" {
		t.Fatalf("projected compaction changed with source: %#v", projected.Compaction)
	}
}

func TestProjectSessionRejectsInvalidEventRelationships(t *testing.T) {
	assistantWithCalls := func(ids ...string) Entry {
		content := make([]llm.AssistantContent, 0, len(ids))
		for _, id := range ids {
			content = append(content, &llm.ToolCall{
				ID: id, Name: "read", Arguments: map[string]any{"path": id},
			})
		}
		return NewMessage(agent.FromLLM(&llm.AssistantMessage{
			Content: content, StopReason: llm.StopReasonToolUse,
		}))
	}
	result := func(id string) Entry {
		return NewMessage(agent.FromLLM(&llm.ToolResultMessage{
			ToolCallID: id, ToolName: "read",
			Content: []llm.ToolResultContent{&llm.TextContent{Text: "done"}},
		}))
	}
	lifecyclePrefix := func() []Entry {
		return []Entry{
			NewRunStart("run-1"),
			NewTurnStart("run-1", "turn-1"),
			NewStepStart("run-1", "turn-1", "step-1"),
		}
	}

	duplicateOne := NewMessage(agent.UserMessage("one"))
	duplicateTwo := NewMessage(agent.UserMessage("two"))
	duplicateTwo.ID = duplicateOne.ID

	tests := []struct {
		name    string
		entries []Entry
		want    string
	}{
		{
			name: "duplicate entry id", entries: []Entry{
				NewRunStart("duplicate-run"),
				NewTurnStart("duplicate-run", "duplicate-turn"),
				duplicateOne,
				duplicateTwo,
			},
			want: "duplicate entry id",
		},
		{
			name: "tool result out of model order",
			entries: append(lifecyclePrefix(),
				assistantWithCalls("call-1", "call-2"), result("call-2")),
			want: "out of model order",
		},
		{
			name: "dispatch arguments differ",
			entries: append(lifecyclePrefix(),
				assistantWithCalls("call-1"),
				NewToolCall(ToolCall{
					ToolCallID: "call-1", ToolName: "read",
					Arguments: json.RawMessage(`{"path":"different"}`),
				}),
			),
			want: "arguments differ",
		},
		{
			name: "outcome precedes result",
			entries: append(lifecyclePrefix(),
				assistantWithCalls("call-1"),
				NewToolOutcome(ToolOutcome{
					ToolCallID: "call-1", Status: agent.ToolOutcomeSuccess,
				}),
			),
			want: "precedes result",
		},
		{
			name: "step ends with unresolved tool",
			entries: append(lifecyclePrefix(),
				assistantWithCalls("call-1"),
				NewStepEnd("run-1", "turn-1", "step-1", LifecycleCompleted, ""),
			),
			want: "follows unresolved tool call",
		},
		{
			name: "compaction references missing message",
			entries: []Entry{NewCompaction(Compaction{
				Summary: "summary", FirstKeptEntryID: "missing",
			})},
			want: "is not a preceding message",
		},
		{
			name: "context without step",
			entries: []Entry{
				NewRunStart("context-run"),
				NewTurnStart("context-run", "context-turn"),
				NewContext(ContextAttachment{
					AttachmentID: "context:1", Epoch: 1, Kind: "base",
					Placement: "prefix", Revision: "revision", Rendered: "context",
				}),
			},
			want: "has no open step",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stampProjectionEntries(test.entries)
			_, err := ProjectSession(test.entries)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ProjectSession() error = %v, want %q", err, test.want)
			}
		})
	}
}

func stampProjectionEntries(entries []Entry) {
	base := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	for index := range entries {
		entries[index].Timestamp = base.Add(time.Duration(index) * time.Second)
	}
}
