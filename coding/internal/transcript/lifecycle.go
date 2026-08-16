package transcript

import (
	"fmt"
)

const LifecycleInterruptedReason = "process_interrupted"

type lifecycleCursor struct {
	runID  string
	turnID string
	stepID string
}

// RepairInterruptedLifecycle validates explicit Run, Turn, and Step nesting.
// When the committed log ends with open boundaries, it returns an interrupted
// terminal tail without changing the supplied prefix.
func RepairInterruptedLifecycle(entries []Entry) ([]Entry, error) {
	var cursor lifecycleCursor
	sawLifecycle := false
	seenRuns := make(map[string]bool)
	seenTurns := make(map[string]bool)
	seenSteps := make(map[string]bool)
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		if !isLifecycleEntry(entry.Type) {
			continue
		}
		sawLifecycle = true
		lifecycle := *entry.Lifecycle
		switch entry.Type {
		case RunStartEntry:
			if seenRuns[lifecycle.RunID] {
				return nil, lifecycleOrderError(entry, "reuses run id %s", lifecycle.RunID)
			}
			if cursor.runID != "" {
				return nil, lifecycleOrderError(entry, "starts while run %s is open", cursor.runID)
			}
			cursor.runID = lifecycle.RunID
			seenRuns[lifecycle.RunID] = true

		case RunEndEntry:
			if err := cursor.requireRun(entry, lifecycle.RunID); err != nil {
				return nil, err
			}
			if cursor.stepID != "" || cursor.turnID != "" {
				return nil, lifecycleOrderError(entry, "ends before its open turn and step")
			}
			cursor = lifecycleCursor{}

		case TurnStartEntry:
			if seenTurns[lifecycle.TurnID] {
				return nil, lifecycleOrderError(entry, "reuses turn id %s", lifecycle.TurnID)
			}
			if err := cursor.requireRun(entry, lifecycle.RunID); err != nil {
				return nil, err
			}
			if cursor.turnID != "" {
				return nil, lifecycleOrderError(entry, "starts while turn %s is open", cursor.turnID)
			}
			cursor.turnID = lifecycle.TurnID
			seenTurns[lifecycle.TurnID] = true

		case TurnEndEntry:
			if err := cursor.requireTurn(entry, lifecycle); err != nil {
				return nil, err
			}
			if cursor.stepID != "" {
				return nil, lifecycleOrderError(entry, "ends while step %s is open", cursor.stepID)
			}
			cursor.turnID = ""

		case StepStartEntry:
			if seenSteps[lifecycle.StepID] {
				return nil, lifecycleOrderError(entry, "reuses step id %s", lifecycle.StepID)
			}
			if err := cursor.requireTurn(entry, lifecycle); err != nil {
				return nil, err
			}
			if cursor.stepID != "" {
				return nil, lifecycleOrderError(entry, "starts while step %s is open", cursor.stepID)
			}
			cursor.stepID = lifecycle.StepID
			seenSteps[lifecycle.StepID] = true

		case StepEndEntry:
			if err := cursor.requireStep(entry, lifecycle); err != nil {
				return nil, err
			}
			cursor.stepID = ""
		}
	}

	if !sawLifecycle || cursor.runID == "" {
		return nil, nil
	}

	var repairs []Entry
	if cursor.stepID != "" {
		repairs = append(repairs, NewStepEnd(
			cursor.runID,
			cursor.turnID,
			cursor.stepID,
			LifecycleInterrupted,
			LifecycleInterruptedReason,
		))
		cursor.stepID = ""
	}
	if cursor.turnID != "" {
		repairs = append(repairs, NewTurnEnd(
			cursor.runID,
			cursor.turnID,
			LifecycleInterrupted,
			LifecycleInterruptedReason,
		))
		cursor.turnID = ""
	}
	runEnd := NewRunEnd(
		cursor.runID,
		LifecycleInterrupted,
		LifecycleInterruptedReason,
	)
	repairs = append(repairs, runEnd)
	return repairs, nil
}

func isLifecycleEntry(entryType EntryType) bool {
	switch entryType {
	case RunStartEntry, RunEndEntry,
		TurnStartEntry, TurnEndEntry,
		StepStartEntry, StepEndEntry:
		return true
	default:
		return false
	}
}

func (c lifecycleCursor) requireRun(entry Entry, runID string) error {
	if c.runID == "" {
		return lifecycleOrderError(entry, "has no open run")
	}
	if c.runID != runID {
		return lifecycleOrderError(entry, "belongs to run %s, want %s", runID, c.runID)
	}
	return nil
}

func (c lifecycleCursor) requireTurn(entry Entry, lifecycle Lifecycle) error {
	if err := c.requireRun(entry, lifecycle.RunID); err != nil {
		return err
	}
	if c.turnID == "" {
		return lifecycleOrderError(entry, "has no open turn")
	}
	if c.turnID != lifecycle.TurnID {
		return lifecycleOrderError(
			entry,
			"belongs to turn %s, want %s",
			lifecycle.TurnID,
			c.turnID,
		)
	}
	return nil
}

func (c lifecycleCursor) requireStep(entry Entry, lifecycle Lifecycle) error {
	if err := c.requireTurn(entry, lifecycle); err != nil {
		return err
	}
	if c.stepID == "" {
		return lifecycleOrderError(entry, "has no open step")
	}
	if c.stepID != lifecycle.StepID {
		return lifecycleOrderError(
			entry,
			"belongs to step %s, want %s",
			lifecycle.StepID,
			c.stepID,
		)
	}
	return nil
}

func lifecycleOrderError(entry Entry, format string, args ...any) error {
	return fmt.Errorf(
		"transcript: lifecycle entry %s (%s) %s",
		entry.ID,
		entry.Type,
		fmt.Sprintf(format, args...),
	)
}
