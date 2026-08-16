package transcript

import (
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
)

func TestRepairInterruptedLifecycleClosesOpenBoundaries(t *testing.T) {
	entries := []Entry{
		NewRunStart("run-1"),
		NewTurnStart("run-1", "turn-1"),
		NewMessage(agent.UserMessage("continue the work")),
		NewStepStart("run-1", "turn-1", "step-1"),
	}

	repairs, err := RepairInterruptedLifecycle(entries)
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := []EntryType{StepEndEntry, TurnEndEntry, RunEndEntry}
	if len(repairs) != len(wantTypes) {
		t.Fatalf("repairs = %#v", repairs)
	}
	for index, want := range wantTypes {
		if repairs[index].Type != want {
			t.Fatalf("repairs[%d].Type = %q, want %q", index, repairs[index].Type, want)
		}
	}
	for _, entry := range repairs {
		if entry.Lifecycle.Status != LifecycleInterrupted ||
			entry.Lifecycle.Reason != LifecycleInterruptedReason {
			t.Fatalf("interrupted boundary = %#v", entry)
		}
	}
	if more, err := RepairInterruptedLifecycle(append(entries, repairs...)); err != nil || len(more) != 0 {
		t.Fatalf("second repair = %#v, %v; want no-op", more, err)
	}
}

func TestRepairInterruptedLifecycleRejectsInvalidNesting(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		want    string
	}{
		{
			name:    "turn without run",
			entries: []Entry{NewTurnStart("run-1", "turn-1")},
			want:    "no open run",
		},
		{
			name: "step with wrong turn",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewStepStart("run-1", "turn-2", "step-1"),
			},
			want: "want turn-1",
		},
		{
			name: "turn ends before step",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewStepStart("run-1", "turn-1", "step-1"),
				NewTurnEnd("run-1", "turn-1", LifecycleCompleted, ""),
			},
			want: "while step step-1 is open",
		},
		{
			name: "second run overlaps",
			entries: []Entry{
				NewRunStart("run-1"),
				NewRunStart("run-2"),
			},
			want: "while run run-1 is open",
		},
		{
			name: "reused step id",
			entries: []Entry{
				NewRunStart("run-1"),
				NewTurnStart("run-1", "turn-1"),
				NewStepStart("run-1", "turn-1", "step-1"),
				NewStepEnd("run-1", "turn-1", "step-1", LifecycleCompleted, ""),
				NewStepStart("run-1", "turn-1", "step-1"),
			},
			want: "reuses step id step-1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := RepairInterruptedLifecycle(test.entries)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RepairInterruptedLifecycle() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLifecycleEntryValidatesShapeAndTerminalStatus(t *testing.T) {
	tests := []Entry{
		newLifecycleEntry(RunStartEntry, Lifecycle{}),
		newLifecycleEntry(TurnStartEntry, Lifecycle{RunID: "run-1"}),
		newLifecycleEntry(StepStartEntry, Lifecycle{RunID: "run-1", TurnID: "turn-1"}),
		newLifecycleEntry(RunStartEntry, Lifecycle{RunID: "run-1", Status: LifecycleCompleted}),
		newLifecycleEntry(RunEndEntry, Lifecycle{RunID: "run-1", Status: "running"}),
	}
	for index, entry := range tests {
		if err := entry.Validate(); err == nil {
			t.Fatalf("invalid lifecycle entry %d passed validation: %#v", index, entry)
		}
	}
}
