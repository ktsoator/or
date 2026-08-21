package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const (
	ToolNameExitPlanMode = "exit_plan_mode"
	planReviewQuestion   = "Approve this plan and leave plan mode?"
	planApproveLabel     = "Approve"
	planContinueLabel    = "Keep planning"
)

// PlanModeState is the session-owned state used by exit_plan_mode. The tool
// reads and changes it without owning persistence or prompt assembly.
type PlanModeState interface {
	PlanModeActive() bool
	ExitPlanMode(context.Context) error
}

// PlanExitOutcome is the structured successful result of exit_plan_mode.
type PlanExitOutcome struct {
	Approved bool `json:"approved"`
}

type exitPlanModeArgs struct {
	Plan string `json:"plan" jsonschema:"description=The complete plan as Markdown starting with a # heading,minLength=1"`
}

// ExitPlanMode presents a completed plan for review. Approval durably leaves
// plan mode before the tool result asks the model to execute the next step.
func ExitPlanMode(asker Asker, state PlanModeState) Tool {
	def := llm.MustTool[exitPlanModeArgs](ToolNameExitPlanMode, exitPlanModeText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Review plan",
			Execute: func(
				ctx context.Context,
				_ string,
				raw json.RawMessage,
				_ func(agent.ToolProgress),
			) (agent.ToolResult, error) {
				var in exitPlanModeArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				plan := strings.TrimSpace(in.Plan)
				if !strings.HasPrefix(plan, "# ") || strings.TrimSpace(strings.TrimPrefix(plan, "# ")) == "" {
					return agent.ToolResult{}, errors.New("exit_plan_mode requires a complete Markdown plan starting with a # heading")
				}
				if asker == nil {
					return agent.ToolResult{}, errors.New("exit_plan_mode requires a user review channel")
				}
				if state == nil || !state.PlanModeActive() {
					return agent.ToolResult{}, errors.New("exit_plan_mode is only available in plan mode")
				}

				answers, err := asker.Ask(ctx, []Question{{
					Question: planReviewQuestion,
					Header:   "Plan review",
					Detail:   plan,
					Intent:   QuestionIntentPlanReview,
					Options: []Option{
						{Label: planApproveLabel, Description: "Leave plan mode and carry out the plan from the next step."},
						{Label: planContinueLabel, Description: "Stay in plan mode and revise the plan."},
					},
				}})
				if err != nil {
					return agent.ToolResult{}, err
				}
				values := alignedAnswerValues(planReviewQuestion, answers)
				if len(values) != 1 || values[0] != planApproveLabel {
					feedback := ""
					if len(values) == 1 && values[0] != planContinueLabel {
						feedback = values[0]
					}
					message := "The user chose to keep planning; revise the plan and present it again."
					if feedback != "" {
						message = fmt.Sprintf("The user chose to keep planning; their feedback: %s", feedback)
					}
					return textResult(message), errors.New(message)
				}
				if err := state.ExitPlanMode(ctx); err != nil {
					return agent.ToolResult{}, fmt.Errorf("exit plan mode: %w", err)
				}
				return resultWith(
					"Plan approved - plan mode exited; carry out the plan starting with your next step.",
					PlanExitOutcome{Approved: true},
				), nil
			},
		},
		AccessFor: InternalAccess,
	}
}

func alignedAnswerValues(question string, answers []Answer) []string {
	for _, answer := range answers {
		if strings.TrimSpace(answer.Question) == question {
			return usableValues(answer.Values)
		}
	}
	return nil
}
