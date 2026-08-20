package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ktsoator/or/coding/internal/contextprojection"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

func TestContextManagerPublishesSkillRevisionOnlyOnCommit(t *testing.T) {
	workspace := t.TempDir()
	oldSkill := skills.Skill{
		Name: "review", Description: "review", Content: "old body",
		Dir: filepath.Join(workspace, "skills", "review"),
	}
	newSkill := oldSkill
	newSkill.Content = "new body"
	loaded := []skills.Skill{oldSkill}
	dynamic := skills.NewDynamicRegistry(skills.NewRegistry(loaded))
	manager := newContextManager(
		workspace,
		"",
		dynamic,
		func() []skills.Skill { return append([]skills.Skill(nil), loaded...) },
		nil,
	)

	loaded = []skills.Skill{newSkill}
	manager.prepareSkillRefresh()
	staged := manager.state()
	if staged.PendingSkillRevision == "" ||
		staged.PendingSkillRevision == staged.SkillRevision ||
		staged.Projection.ActiveSkillsRevision != "" ||
		staged.Projection.StagedSkillsRevision != staged.PendingSkillRevision {
		t.Fatalf("staged state = %#v", staged)
	}
	if current, ok := dynamic.Lookup("review"); !ok || current.Content != "old body" {
		t.Fatalf("registry published before commit: %#v, %v", current, ok)
	}

	prepared := manager.prepareStep(llm.Context{Messages: []llm.Message{llm.UserText("question")}})
	beforeCommit := manager.state()
	if beforeCommit.SkillRevision != staged.SkillRevision ||
		beforeCommit.PendingSkillRevision != staged.PendingSkillRevision {
		t.Fatalf("prepare advanced revision state: before %#v, after %#v", staged, beforeCommit)
	}

	manager.commit(prepared)
	committed := manager.state()
	if committed.SkillRevision != staged.PendingSkillRevision ||
		committed.PendingSkillRevision != "" ||
		committed.Projection.ActiveSkillsRevision != staged.PendingSkillRevision ||
		committed.Projection.StagedSkillsRevision != "" {
		t.Fatalf("committed state = %#v", committed)
	}
	if current, ok := dynamic.Lookup("review"); !ok || current.Content != "new body" {
		t.Fatalf("registry was not published with commit: %#v, %v", current, ok)
	}
}

func TestContextManagerCancelsRevertedContextRefresh(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("old rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := newContextManager(
		workspace,
		"",
		skills.NewDynamicRegistry(skills.NewRegistry(nil)),
		nil,
		nil,
	)
	original := manager.state().ContextRevision

	if err := os.WriteFile(agentsPath, []byte("new rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager.prepareContextRefresh()
	staged := manager.state()
	if staged.PendingContextRevision == "" ||
		staged.PendingContextRevision == original ||
		staged.ContextRevision != original ||
		staged.Projection.StagedContextRevision != staged.PendingContextRevision {
		t.Fatalf("staged state = %#v", staged)
	}

	if err := os.WriteFile(agentsPath, []byte("old rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager.prepareContextRefresh()
	reverted := manager.state()
	if reverted.ContextRevision != original ||
		reverted.PendingContextRevision != "" ||
		reverted.Projection.StagedContextRevision != "" {
		t.Fatalf("reverted state = %#v", reverted)
	}
}

func TestContextManagerRestoresActivatedSkillsInNextEpoch(t *testing.T) {
	entries := []transcript.Entry{{
		Type: transcript.ContextEntry,
		Context: &transcript.ContextAttachment{
			AttachmentID: "activated_skill:4:review:revision",
			Epoch:        4,
			Kind:         string(contextprojection.ActivatedSkill),
			Placement:    string(contextprojection.AfterCurrent),
			Path:         "review",
			Revision:     "revision",
			Rendered:     "review instructions",
		},
	}}
	manager := newContextManager(
		t.TempDir(),
		"",
		skills.NewDynamicRegistry(skills.NewRegistry(nil)),
		nil,
		entries,
	)

	state := manager.state().Projection
	if state.Epoch != 5 || state.ActivatedSkillCount != 1 || state.PendingSkillCount != 0 {
		t.Fatalf("restored state = %#v", state)
	}
	prepared := manager.prepareStep(llm.Context{})
	for _, pending := range prepared.Pending {
		if pending.Kind == contextprojection.ActivatedSkill {
			t.Fatalf("restored activated skill became pending: %#v", pending)
		}
	}
}
