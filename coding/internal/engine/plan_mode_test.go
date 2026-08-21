package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type approvingPlanAsker struct {
	questions []tools.Question
}

func (a *approvingPlanAsker) Ask(_ context.Context, questions []tools.Question) ([]tools.Answer, error) {
	a.questions = append([]tools.Question(nil), questions...)
	return []tools.Answer{{
		Question: questions[0].Question,
		Values:   []string{"Approve"},
	}}, nil
}

func TestSessionPlanModePersistsAndRestores(t *testing.T) {
	ctx := context.Background()
	store := &transcript.Memory{}
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	var changed []bool
	session.Subscribe(func(event Event) {
		if event.Type == PlanModeChanged {
			changed = append(changed, event.PlanMode)
		}
	})
	if err := session.SetPlanMode(ctx, true); err != nil {
		t.Fatal(err)
	}
	if !session.PlanModeActive() {
		t.Fatal("live session did not enter plan mode")
	}
	if !strings.Contains(session.Snapshot().SystemPrompt, "## Plan mode") {
		t.Fatal("live session prompt has no plan-mode policy")
	}
	if len(changed) != 1 || !changed[0] {
		t.Fatalf("plan-mode events = %v, want [true]", changed)
	}
	session.Close()

	restored, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !restored.PlanModeActive() {
		t.Fatal("restored session lost plan mode")
	}
	if !strings.Contains(restored.Snapshot().SystemPrompt, "## Plan mode") {
		t.Fatal("restored session prompt has no plan-mode policy")
	}

	entries, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != transcript.PlanModeEntry ||
		entries[0].PlanMode == nil || !entries[0].PlanMode.Active {
		t.Fatalf("persisted plan mode = %#v", entries)
	}
}

func TestApprovedPlanRefreshesTheNextModelStep(t *testing.T) {
	ctx := context.Background()
	store := &transcript.Memory{}
	asker := &approvingPlanAsker{}
	var requests []llm.Context
	modelCalls := 0
	session, err := New(ctx, Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		Asker: asker,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			requests = append(requests, input)
			modelCalls++
			if modelCalls == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{&llm.ToolCall{
					ID:   "call-exit-plan",
					Name: tools.ToolNameExitPlanMode,
					Arguments: map[string]any{
						"plan": "# Implementation plan\n\n1. Change the implementation.\n2. Run tests.",
					},
				}}
				return finalEvents(llm.EventDone, &message), nil
			}
			return assistantEvents(model, "implemented"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var changed []bool
	session.Subscribe(func(event Event) {
		if event.Type == PlanModeChanged {
			changed = append(changed, event.PlanMode)
		}
	})
	if err := session.SetPlanMode(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(ctx, "design this change"); err != nil {
		t.Fatal(err)
	}

	if modelCalls != 2 || len(requests) != 2 {
		t.Fatalf("model requests = %d, captured contexts = %d; want two", modelCalls, len(requests))
	}
	if !strings.Contains(requests[0].SystemPrompt, "## Plan mode") {
		t.Fatal("planning request did not include the plan-mode policy")
	}
	if strings.Contains(requests[1].SystemPrompt, "## Plan mode") {
		t.Fatal("post-approval request still included the plan-mode policy")
	}
	if session.PlanModeActive() {
		t.Fatal("approved session remained in plan mode")
	}
	if len(changed) != 2 || !changed[0] || changed[1] {
		t.Fatalf("plan-mode events = %v, want [true false]", changed)
	}
	if len(asker.questions) != 1 || asker.questions[0].Intent != tools.QuestionIntentPlanReview ||
		!strings.HasPrefix(asker.questions[0].Detail, "# Implementation plan") {
		t.Fatalf("plan review = %#v", asker.questions)
	}

	entries, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var modes []bool
	for _, entry := range entries {
		if entry.Type == transcript.PlanModeEntry && entry.PlanMode != nil {
			modes = append(modes, entry.PlanMode.Active)
		}
	}
	if len(modes) != 2 || !modes[0] || modes[1] {
		t.Fatalf("persisted plan-mode transitions = %v, want [true false]", modes)
	}
}
