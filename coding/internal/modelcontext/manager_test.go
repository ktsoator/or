package modelcontext

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestPrepareStepPrependsBaseWithoutMutatingCanonicalInput(t *testing.T) {
	manager := New(3, "", `<or-context kind="base">rules</or-context>`, "", "")
	canonical := llm.Context{
		SystemPrompt: "stable",
		Messages:     []llm.Message{llm.UserText("question")},
		Tools:        []llm.ToolDefinition{{Name: "read"}},
	}

	prepared := manager.PrepareStep(canonical)
	if prepared.Input.SystemPrompt != "stable" {
		t.Fatalf("system prompt = %q, want stable", prepared.Input.SystemPrompt)
	}
	if len(prepared.Input.Messages) != 2 {
		t.Fatalf("projected messages = %d, want context and user", len(prepared.Input.Messages))
	}
	if got := userText(t, prepared.Input.Messages[0]); got != `<or-context kind="base">rules</or-context>` {
		t.Fatalf("base context = %q", got)
	}
	if got := userText(t, prepared.Input.Messages[1]); got != "question" {
		t.Fatalf("canonical user = %q", got)
	}
	if len(canonical.Messages) != 1 || userText(t, canonical.Messages[0]) != "question" {
		t.Fatal("PrepareStep mutated canonical messages")
	}
	if len(prepared.Pending) != 1 || prepared.Pending[0].Epoch != 3 {
		t.Fatalf("pending attachments = %#v", prepared.Pending)
	}
}

func TestCommitStopsRepersistingButKeepsProjectingBase(t *testing.T) {
	manager := New(1, "", "base", "", "")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}

	first := manager.PrepareStep(canonical)
	manager.Commit(first)
	second := manager.PrepareStep(canonical)

	if len(second.Pending) != 0 {
		t.Fatalf("pending after commit = %#v", second.Pending)
	}
	if len(second.Input.Messages) != 2 || userText(t, second.Input.Messages[0]) != "base" {
		t.Fatalf("base context stopped projecting after commit: %#v", second.Input.Messages)
	}
	state := manager.State()
	if !state.HasBase || !state.BaseCommitted || state.Epoch != 1 || state.BaseRevision == "" {
		t.Fatalf("state = %#v", state)
	}
}

func TestEmptyBaseLeavesInputUnchanged(t *testing.T) {
	manager := New(2, "", "", "", "")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}
	prepared := manager.PrepareStep(canonical)
	if len(prepared.Input.Messages) != 1 || len(prepared.Pending) != 0 {
		t.Fatalf("prepared = %#v", prepared)
	}
	state := manager.State()
	if state.HasBase || !state.BaseCommitted || state.Epoch != 2 {
		t.Fatalf("state = %#v", state)
	}
}

func TestSkillListingAndLatestUpdateUseStablePlacements(t *testing.T) {
	manager := New(4, "", "base", "skills-v1", "initial skills")
	canonical := llm.Context{
		Messages: []llm.Message{
			llm.UserText("question"),
			llm.AssistantText("answer"),
		},
	}

	first := manager.PrepareStep(canonical)
	if got := messageTexts(t, first.Input.Messages); !equalStrings(
		got,
		[]string{"base", "initial skills", "question", "answer"},
	) {
		t.Fatalf("initial projection = %v", got)
	}
	if len(first.Pending) != 2 ||
		first.Pending[0].Kind != BaseContext ||
		first.Pending[1].Kind != SkillListing {
		t.Fatalf("initial pending = %#v", first.Pending)
	}
	manager.Commit(first)

	manager.StageSkillsUpdate("skills-v2", "updated skills")
	update := manager.PrepareStep(canonical)
	if got := messageTexts(t, update.Input.Messages); !equalStrings(
		got,
		[]string{"base", "initial skills", "question", "answer", "updated skills"},
	) {
		t.Fatalf("updated projection = %v", got)
	}
	if len(update.Pending) != 1 ||
		update.Pending[0].Kind != SkillsUpdate ||
		update.Pending[0].Placement != AfterCurrent {
		t.Fatalf("update pending = %#v", update.Pending)
	}
	manager.Commit(update)

	retry := manager.PrepareStep(canonical)
	if len(retry.Pending) != 0 {
		t.Fatalf("retry pending = %#v", retry.Pending)
	}
	if got := messageTexts(t, retry.Input.Messages); !equalStrings(
		got,
		[]string{"base", "initial skills", "question", "answer", "updated skills"},
	) {
		t.Fatalf("retry projection = %v", got)
	}

	manager.StageSkillsUpdate("skills-v3", "latest skills")
	latest := manager.PrepareStep(canonical)
	if got := messageTexts(t, latest.Input.Messages); !equalStrings(
		got,
		[]string{"base", "initial skills", "question", "answer", "latest skills"},
	) {
		t.Fatalf("latest projection retained obsolete update: %v", got)
	}
}

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

func TestContextAndSkillUpdatesProjectTogether(t *testing.T) {
	manager := New(8, "base-v1", "base", "skills-v1", "listing")
	canonical := llm.Context{Messages: []llm.Message{llm.UserText("question")}}
	manager.Commit(manager.PrepareStep(canonical))

	manager.StageContextUpdate("base-v2", "context update")
	manager.StageSkillsUpdate("skills-v2", "skills update")
	prepared := manager.PrepareStep(canonical)
	if got := messageTexts(t, prepared.Input.Messages); !equalStrings(
		got,
		[]string{"base", "listing", "question", "context update", "skills update"},
	) {
		t.Fatalf("combined projection = %v", got)
	}
	manager.Commit(prepared)
	if len(manager.PrepareStep(canonical).Pending) != 0 {
		t.Fatal("committed updates were re-emitted as pending")
	}
}

func userText(t *testing.T, message llm.Message) string {
	t.Helper()
	user, ok := message.(*llm.UserMessage)
	if !ok {
		t.Fatalf("message = %T, want user", message)
	}
	var text string
	for _, content := range user.Content {
		if block, ok := content.(*llm.TextContent); ok {
			text += block.Text
		}
	}
	return text
}

func messageTexts(t *testing.T, messages []llm.Message) []string {
	t.Helper()
	result := make([]string, len(messages))
	for index, message := range messages {
		switch typed := message.(type) {
		case *llm.UserMessage:
			result[index] = userText(t, typed)
		case *llm.AssistantMessage:
			for _, content := range typed.Content {
				if block, ok := content.(*llm.TextContent); ok {
					result[index] += block.Text
				}
			}
		default:
			t.Fatalf("message %d = %T", index, message)
		}
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
