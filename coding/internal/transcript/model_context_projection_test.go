package transcript

import (
	"reflect"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

func TestIncrementalModelContextProjectionMatchesBuildContextReplay(t *testing.T) {
	firstRun, _ := completedModelContextRun("one", "old user", "old assistant")
	secondRun, keptUser := completedModelContextRun("two", "kept user", "kept assistant")
	firstCompaction := NewCompaction(Compaction{
		Summary:          "first summary",
		FirstKeptEntryID: keptUser.ID,
	})
	thirdRun, newestUser := completedModelContextRun("three", "new user", "new assistant")
	secondCompaction := NewCompaction(Compaction{
		Summary:          "second summary",
		FirstKeptEntryID: newestUser.ID,
	})
	fourthRun, _ := completedModelContextRun("four", "latest user", "latest assistant")
	entries := append(firstRun, secondRun...)
	entries = append(entries, firstCompaction)
	entries = append(entries, thirdRun...)
	entries = append(entries, secondCompaction)
	entries = append(entries, fourthRun...)
	entries = sequencedForTest(entries...)

	registry := NewProjectionRegistry()
	unit := NewModelContextProjectionUnit()
	if err := registry.Register(unit); err != nil {
		t.Fatal(err)
	}
	validator, err := validateSession(nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	for start := 0; start < len(entries); {
		batchSize := start%5 + 1
		end := min(start+batchSize, len(entries))
		prepared, err := validator.PrepareAppend(entries[start:end])
		if err != nil {
			t.Fatal(err)
		}
		prepared.Commit()

		incremental, err := unit.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := BuildContext(entries[:end])
		if err != nil {
			t.Fatal(err)
		}
		if incremental.AppliedEntries != end || incremental.AsOfSeq != int64(end-1) {
			t.Fatalf("projection boundary after %d entries = %#v", end, incremental)
		}
		if !reflect.DeepEqual(incremental.Messages, replayed) {
			t.Fatalf(
				"incremental model context differs after %d entries\nincremental: %#v\nreplayed: %#v",
				end,
				incremental.Messages,
				replayed,
			)
		}
		start = end
	}

	projected, err := ProjectModelContext(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected.Messages, mustBuildContext(t, entries)) ||
		projected.ActiveCompactionEntryID != secondCompaction.ID ||
		projected.FirstKeptEntryID != newestUser.ID {
		t.Fatalf("complete model-context projection = %#v", projected)
	}
}

func TestModelContextProjectionSnapshotsAreDetached(t *testing.T) {
	entries, _ := completedModelContextRun("one", "original", "answer")
	entries = sequencedForTest(entries...)
	unit := NewModelContextProjectionUnit()
	registry := NewProjectionRegistry()
	if err := registry.Register(unit); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSession(entries, registry); err != nil {
		t.Fatal(err)
	}

	first, err := unit.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	message, _ := agent.ToLLM(first.Messages[0])
	message.(*llm.UserMessage).Content[0].(*llm.TextContent).Text = "mutated"

	second, err := unit.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := messageText(t, second.Messages[0]); got != "original" {
		t.Fatalf("live model context changed through snapshot: %q", got)
	}
}

func completedModelContextRun(
	suffix string,
	userText string,
	assistantText string,
) ([]Entry, Entry) {
	runID := "run-" + suffix
	turnID := "turn-" + suffix
	stepID := "step-" + suffix
	user := NewMessage(agent.UserMessage(userText))
	return []Entry{
		NewRunStart(runID),
		NewTurnStart(runID, turnID),
		user,
		NewStepStart(runID, turnID, stepID),
		NewContext(ContextAttachment{
			AttachmentID: "context-" + suffix,
			Epoch:        1,
			Kind:         "base",
			Placement:    "prefix",
			Revision:     "revision-" + suffix,
			Rendered:     "hidden context " + suffix,
		}),
		NewMessage(agent.FromLLM(assistant(assistantText))),
		NewStepEnd(runID, turnID, stepID, LifecycleCompleted, ""),
		NewTurnEnd(runID, turnID, LifecycleCompleted, ""),
		NewRunEnd(runID, LifecycleCompleted, ""),
	}, user
}

func mustBuildContext(t *testing.T, entries []Entry) []agent.AgentMessage {
	t.Helper()
	messages, err := BuildContext(entries)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}
