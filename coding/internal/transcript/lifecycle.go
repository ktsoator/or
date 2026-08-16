package transcript

import "fmt"

const LifecycleInterruptedReason = "process_interrupted"

// RepairInterruptedLifecycle validates the complete event prefix through the
// canonical reducer. When the prefix ends inside a lifecycle, it returns the
// interrupted terminal boundaries without mutating the supplied entries.
func RepairInterruptedLifecycle(entries []Entry) ([]Entry, error) {
	reducer := newSessionReducer(len(entries))
	for index, entry := range entries {
		if _, err := reducer.Apply(index, entry); err != nil {
			return nil, err
		}
	}
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
