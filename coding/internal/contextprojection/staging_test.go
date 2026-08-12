package contextprojection

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestCancelStagedSkillsUpdateKeepsCommittedSnapshot(t *testing.T) {
	manager := New(1, "", "", "", "")
	manager.StageSkillsUpdate("v1", "first")
	first := manager.PrepareStep(llm.Context{})
	manager.Commit(first)

	manager.StageSkillsUpdate("v2", "second")
	manager.CancelStagedSkillsUpdate()
	prepared := manager.PrepareStep(llm.Context{})
	if got := messageTexts(t, prepared.Input.Messages); !equalStrings(got, []string{"first"}) {
		t.Fatalf("projection after cancel = %v", got)
	}
	if len(prepared.Pending) != 0 {
		t.Fatalf("pending after cancel = %#v", prepared.Pending)
	}
}

func TestActivatedSkillsRemainProjectedAndDeduplicateByName(t *testing.T) {
	manager := New(5, "", "", "", "")
	manager.StageActivatedSkill("review", "v1", "review body v1")
	manager.StageActivatedSkill("review", "v2", "review body v2")
	manager.StageActivatedSkill("build", "v1", "build body")

	first := manager.PrepareStep(llm.Context{Messages: []llm.Message{llm.UserText("task")}})
	if got := messageTexts(t, first.Input.Messages); !equalStrings(
		got,
		[]string{"task", "build body", "review body v1"},
	) {
		t.Fatalf("activated Skill projection = %v", got)
	}
	if len(first.Pending) != 2 || first.Pending[0].Kind != ActivatedSkill ||
		first.Pending[0].Path != "build" || first.Pending[1].Path != "review" {
		t.Fatalf("pending activated Skills = %#v", first.Pending)
	}
	manager.Commit(first)

	retry := manager.PrepareStep(llm.Context{Messages: []llm.Message{llm.UserText("next")}})
	if len(retry.Pending) != 0 {
		t.Fatalf("committed activated Skills became pending again: %#v", retry.Pending)
	}
	if got := messageTexts(t, retry.Input.Messages); !equalStrings(
		got,
		[]string{"next", "build body", "review body v1"},
	) {
		t.Fatalf("activated Skills stopped projecting = %v", got)
	}
	state := manager.State()
	if state.ActivatedSkillCount != 2 || state.PendingSkillCount != 0 {
		t.Fatalf("state = %#v", state)
	}
}

func TestRestoreActivatedSkillsKeepsDurableSnapshot(t *testing.T) {
	manager := New(8, "", "", "", "")
	manager.RestoreActivatedSkills([]Attachment{{
		ID: "activated_skill:3:v1", Epoch: 3, Kind: ActivatedSkill,
		Placement: AfterCurrent, Path: "review", Revision: "v1", Rendered: "saved body",
	}})
	manager.StageActivatedSkill("review", "v2", "new body")

	prepared := manager.PrepareStep(llm.Context{})
	if len(prepared.Pending) != 0 {
		t.Fatalf("restored activation was not durable: %#v", prepared.Pending)
	}
	if got := messageTexts(t, prepared.Input.Messages); !equalStrings(got, []string{"saved body"}) {
		t.Fatalf("restored activated Skill = %v", got)
	}
}

func TestContextUpdateSupersedesBaseWithoutDisturbingThePrefix(t *testing.T) {
	manager := New(7, "base-v1", "base", "", "")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}
	manager.Commit(manager.PrepareStep(canonical))

	// Restaging the state the model already sees must not emit a block.
	manager.StageContextUpdate("base-v1", "identical")
	unchanged := manager.PrepareStep(canonical)
	if got := messageTexts(t, unchanged.Input.Messages); !equalStrings(
		got,
		[]string{"base", "question"},
	) {
		t.Fatalf("unchanged projection = %v", got)
	}

	manager.StageContextUpdate("base-v2", "new environment and rules")
	updated := manager.PrepareStep(canonical)
	if got := messageTexts(t, updated.Input.Messages); !equalStrings(
		got,
		[]string{"base", "question", "new environment and rules"},
	) {
		t.Fatalf("updated projection = %v", got)
	}
	// The cached request prefix must survive the change untouched.
	if got := userText(t, updated.Input.Messages[0]); got != "base" {
		t.Fatalf("context update rewrote the prefix: %q", got)
	}
	if len(updated.Pending) != 1 ||
		updated.Pending[0].Kind != ContextUpdate ||
		updated.Pending[0].Placement != AfterCurrent {
		t.Fatalf("update pending = %#v", updated.Pending)
	}
	manager.Commit(updated)

	// A later revision replaces the previous update rather than stacking on it.
	manager.StageContextUpdate("base-v3", "newest rules")
	latest := manager.PrepareStep(canonical)
	if got := messageTexts(t, latest.Input.Messages); !equalStrings(
		got,
		[]string{"base", "question", "newest rules"},
	) {
		t.Fatalf("latest projection retained an obsolete update: %v", got)
	}
	manager.CancelStagedContextUpdate()
	if got := messageTexts(t, manager.PrepareStep(canonical).Input.Messages); !equalStrings(
		got,
		[]string{"base", "question", "new environment and rules"},
	) {
		t.Fatalf("cancel dropped the committed update: %v", got)
	}
	state := manager.State()
	if state.ActiveContextRevision != "base-v2" || state.StagedContextRevision != "" {
		t.Fatalf("state = %#v", state)
	}
}

func TestTaskStatusSnapshotReplacesPreviousSnapshot(t *testing.T) {
	manager := New(9, "", "base", "", "")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}
	manager.Commit(manager.PrepareStep(canonical))

	manager.StageTaskStatus("tasks: one")
	first := manager.PrepareStep(canonical)
	if got := messageTexts(t, first.Input.Messages); !equalStrings(
		got,
		[]string{"base", "question", "tasks: one"},
	) {
		t.Fatalf("first task projection = %v", got)
	}
	if len(first.Pending) != 1 || first.Pending[0].Kind != TaskStatus {
		t.Fatalf("task pending = %#v", first.Pending)
	}
	manager.Commit(first)

	manager.StageTaskStatus("tasks: one and two")
	latest := manager.PrepareStep(canonical)
	if got := messageTexts(t, latest.Input.Messages); !equalStrings(
		got,
		[]string{"base", "question", "tasks: one and two"},
	) {
		t.Fatalf("latest task projection retained obsolete snapshot: %v", got)
	}
	state := manager.State()
	if state.ActiveTaskRevision == "" || state.StagedTaskRevision == "" {
		t.Fatalf("task state = %#v", state)
	}
}
