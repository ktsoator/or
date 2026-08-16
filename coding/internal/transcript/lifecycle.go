package transcript

import "fmt"

const LifecycleInterruptedReason = "process_interrupted"

// interruptedLifecycleRepairs returns terminal boundaries for the open scope
// in an already validated reducer state.
func interruptedLifecycleRepairs(reducer *sessionReducer) ([]Entry, error) {
	if len(reducer.pendingTools) > 0 {
		return nil, fmt.Errorf(
			"transcript: cannot close lifecycle with unresolved tool call %s",
			reducer.pendingTools[0],
		)
	}

	scope := reducer.scope
	if scope.RunID == "" {
		return nil, nil
	}

	var repairs []Entry
	if scope.StepID != "" {
		repairs = append(repairs, NewStepEnd(
			scope.RunID,
			scope.TurnID,
			scope.StepID,
			LifecycleInterrupted,
			LifecycleInterruptedReason,
		))
	}
	if scope.TurnID != "" {
		repairs = append(repairs, NewTurnEnd(
			scope.RunID,
			scope.TurnID,
			LifecycleInterrupted,
			LifecycleInterruptedReason,
		))
	}
	repairs = append(repairs, NewRunEnd(
		scope.RunID,
		LifecycleInterrupted,
		LifecycleInterruptedReason,
	))
	return repairs, nil
}
