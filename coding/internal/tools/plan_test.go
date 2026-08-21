package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type stubPlanModeState struct {
	active    bool
	exitCalls int
	exitErr   error
}

func (s *stubPlanModeState) PlanModeActive() bool { return s.active }

func (s *stubPlanModeState) ExitPlanMode(context.Context) error {
	s.exitCalls++
	if s.exitErr != nil {
		return s.exitErr
	}
	s.active = false
	return nil
}

func planArgs(t *testing.T, plan string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(exitPlanModeArgs{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestExitPlanModeRejectsCallsOutsidePlanMode(t *testing.T) {
	state := &stubPlanModeState{}
	result, err := execute(t, ExitPlanMode(&stubAsker{}, state), planArgs(t, "# Plan\n\nDo the work."))
	if err == nil || !strings.Contains(err.Error(), "only available in plan mode") {
		t.Fatalf("error = %v, want inactive plan mode rejection", err)
	}
	if state.exitCalls != 0 || len(result.Content) != 0 {
		t.Fatalf("inactive call changed state or produced a result: calls=%d result=%#v", state.exitCalls, result)
	}
}

func TestExitPlanModeRequiresAHeadingBeforeReview(t *testing.T) {
	asker := &stubAsker{}
	state := &stubPlanModeState{active: true}
	_, err := execute(t, ExitPlanMode(asker, state), planArgs(t, "Inspect the code, then edit it."))
	if err == nil || !strings.Contains(err.Error(), "starting with a # heading") {
		t.Fatalf("error = %v, want Markdown heading rejection", err)
	}
	if len(asker.questions) != 0 || state.exitCalls != 0 {
		t.Fatalf("invalid plan reached review or changed state: questions=%#v calls=%d", asker.questions, state.exitCalls)
	}
}

func TestExitPlanModeApprovalLeavesPlanMode(t *testing.T) {
	asker := &stubAsker{answers: []Answer{{
		Question: planReviewQuestion,
		Values:   []string{planApproveLabel},
	}}}
	state := &stubPlanModeState{active: true}
	result, err := execute(t, ExitPlanMode(asker, state), planArgs(t, "# Plan\n\n1. Change the code.\n2. Run tests."))
	if err != nil {
		t.Fatal(err)
	}
	if state.active || state.exitCalls != 1 {
		t.Fatalf("state after approval = active %v, calls %d", state.active, state.exitCalls)
	}
	if len(asker.questions) != 1 || asker.questions[0].Intent != QuestionIntentPlanReview ||
		!strings.HasPrefix(asker.questions[0].Detail, "# Plan") {
		t.Fatalf("review question = %#v", asker.questions)
	}
	if !strings.Contains(resultText(t, result), "starting with your next step") {
		t.Fatalf("result = %q, want next-step execution instruction", resultText(t, result))
	}
	outcome, ok := result.Outcome.Data.(PlanExitOutcome)
	if !ok || !outcome.Approved {
		t.Fatalf("outcome = %#v, want approved PlanExitOutcome", result.Outcome.Data)
	}
}

func TestExitPlanModeKeepsPlanningAndReturnsFeedback(t *testing.T) {
	asker := &stubAsker{answers: []Answer{{
		Question: planReviewQuestion,
		Values:   []string{"Add a rollback step"},
	}}}
	state := &stubPlanModeState{active: true}
	result, err := execute(t, ExitPlanMode(asker, state), planArgs(t, "# Plan\n\n1. Change the code."))
	if err == nil || !strings.Contains(err.Error(), "Add a rollback step") {
		t.Fatalf("error = %v, want user feedback", err)
	}
	if !state.active || state.exitCalls != 0 {
		t.Fatalf("keep planning changed state: active %v, calls %d", state.active, state.exitCalls)
	}
	if !strings.Contains(resultText(t, result), "Add a rollback step") {
		t.Fatalf("model-facing result = %q, want user feedback", resultText(t, result))
	}
}

func TestExitPlanModePropagatesStateFailure(t *testing.T) {
	asker := &stubAsker{answers: []Answer{{
		Question: planReviewQuestion,
		Values:   []string{planApproveLabel},
	}}}
	want := errors.New("persist failed")
	state := &stubPlanModeState{active: true, exitErr: want}
	_, err := execute(t, ExitPlanMode(asker, state), planArgs(t, "# Plan\n\n1. Change the code."))
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want state failure", err)
	}
	if !state.active || state.exitCalls != 1 {
		t.Fatalf("failed exit changed state unexpectedly: active %v, calls %d", state.active, state.exitCalls)
	}
}
