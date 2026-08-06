package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/modelcontext"
	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/coding/internal/transcript"
	"github.com/ktsoator/or/llm"
)

type mutableSkills struct {
	mu      sync.Mutex
	current []skills.Skill
	calls   int
}

func (m *mutableSkills) load() []skills.Skill {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return append([]skills.Skill(nil), m.current...)
}

func (m *mutableSkills) set(current ...skills.Skill) {
	m.mu.Lock()
	m.current = append([]skills.Skill(nil), current...)
	m.mu.Unlock()
}

func (m *mutableSkills) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestStableSkillToolExistsWithZeroSkills(t *testing.T) {
	var captured llm.Context
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			captured = input
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	state := session.agent.Snapshot()
	if len(state.Tools) != 1 || state.Tools[0].Definition.Name != skills.ToolName {
		t.Fatalf("agent tools = %#v, want stable skill tool", state.Tools)
	}
	if !strings.Contains(state.SystemPrompt, "## Skills") {
		t.Fatalf("stable system prompt omitted skill protocol:\n%s", state.SystemPrompt)
	}
	if err := session.Prompt(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Name != skills.ToolName {
		t.Fatalf("provider tools = %#v, want skill", captured.Tools)
	}
	// Base context, then the user message. No skill listing is attached when no
	// skill is installed.
	if len(captured.Messages) != 2 || llmUserText(t, captured.Messages[1]) != "question" {
		t.Fatalf("zero-skill provider messages = %#v", captured.Messages)
	}
	if base := llmUserText(t, captured.Messages[0]); strings.Contains(base, "skill") {
		t.Fatalf("zero-skill session advertised a skill listing:\n%s", base)
	}
}

func TestDynamicSkillsAddUpdateAndRemoveAtTopLevelBoundary(t *testing.T) {
	ctx := context.Background()
	loader := &mutableSkills{}
	store := &checkpointStore{}
	var inputs []llm.Context
	session, err := New(ctx, Options{
		Model:       llm.Model{Provider: "test", ID: "model"},
		Tools:       []tools.Tool{},
		Store:       store,
		SkillLoader: loader.load,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			inputs = append(inputs, input)
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || len(inputs[0].Messages) != 2 {
		t.Fatalf("initial input = %#v, want base context and user message", inputs)
	}
	stableTools := append([]llm.ToolDefinition(nil), inputs[0].Tools...)

	loader.set(skills.Skill{
		Name: "review", Description: "review changes", Content: "version one", Dir: "/skills/review",
	})
	if err := session.Prompt(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	added := llmUserText(t, inputs[1].Messages[len(inputs[1].Messages)-1])
	for _, want := range []string{
		`kind="skills_update"`,
		"<added>",
		"<name>review</name>",
		"review changes",
	} {
		if !strings.Contains(added, want) {
			t.Errorf("add update missing %q:\n%s", want, added)
		}
	}
	if got := executeSkill(t, session, "review"); !strings.Contains(got, "version one") {
		t.Fatalf("added skill tool result = %q", got)
	}

	loader.set(skills.Skill{
		Name: "review", Description: "review changes", Content: "version two", Dir: "/skills/review",
	})
	if err := session.Prompt(ctx, "third"); err != nil {
		t.Fatal(err)
	}
	updated := llmUserText(t, inputs[2].Messages[len(inputs[2].Messages)-1])
	if !strings.Contains(updated, "<updated>") ||
		!strings.Contains(updated, "<name>review</name>") {
		t.Fatalf("body-only update was not announced:\n%s", updated)
	}
	if got := executeSkill(t, session, "review"); !strings.Contains(got, "version two") {
		t.Fatalf("updated skill tool result = %q", got)
	}

	loader.set()
	if err := session.Prompt(ctx, "fourth"); err != nil {
		t.Fatal(err)
	}
	removed := llmUserText(t, inputs[3].Messages[len(inputs[3].Messages)-1])
	if !strings.Contains(removed, "<removed>") ||
		!strings.Contains(removed, "<name>review</name>") ||
		!strings.Contains(removed, `<available-skills none="true" />`) {
		t.Fatalf("removal update is incomplete:\n%s", removed)
	}
	if _, err := executeSkillResult(session, "review"); err == nil {
		t.Fatal("removed skill still resolves")
	}

	if loader.callCount() != 5 {
		t.Fatalf("loader calls = %d, want construction plus four top-level runs", loader.callCount())
	}
	for index, input := range inputs {
		if !reflect.DeepEqual(input.Tools, stableTools) {
			t.Fatalf("tool schema changed on request %d:\nfirst: %#v\nnext: %#v", index+1, stableTools, input.Tools)
		}
	}

	entries, _, _ := store.snapshot()
	var updates, bases int
	for _, entry := range entries {
		if entry.Type != transcript.ContextEntry || entry.Context == nil {
			continue
		}
		switch entry.Context.Kind {
		case string(modelcontext.BaseContext):
			bases++
		case string(modelcontext.SkillsUpdate):
			updates++
			if entry.Context.Placement != string(modelcontext.AfterCurrent) {
				t.Fatalf("skill context entry = %#v", entry)
			}
		default:
			t.Fatalf("unexpected context entry = %#v", entry)
		}
	}
	if updates != 3 {
		t.Fatalf("durable skill updates = %d, want add, update, remove", updates)
	}
	if bases != 1 {
		t.Fatalf("durable base contexts = %d, want one per epoch", bases)
	}
	for _, item := range session.History() {
		if strings.Contains(item.Text, "<or-context") {
			t.Fatalf("hidden skill update leaked into history: %#v", item)
		}
	}
}

func TestSkillRefreshIsFrozenAcrossAppRetry(t *testing.T) {
	ctx := context.Background()
	loader := &mutableSkills{}
	loader.set(skills.Skill{
		Name: "review", Description: "review", Content: "old body", Dir: "/skills/review",
	})
	var inputs []llm.Context
	session, err := New(ctx, Options{
		Model:       llm.Model{Provider: "test", ID: "model"},
		Tools:       []tools.Tool{},
		SkillLoader: loader.load,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			input llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			inputs = append(inputs, input)
			if len(inputs) == 1 {
				loader.set(skills.Skill{
					Name: "review", Description: "review", Content: "new body", Dir: "/skills/review",
				})
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonError
				message.ErrorMessage = "temporarily unavailable"
				return finalEvents(llm.EventError, &message), nil
			}
			return assistantEvents(model, "recovered"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(ctx, "retry"); err != nil {
		t.Fatal(err)
	}
	if loader.callCount() != 2 {
		t.Fatalf("loader calls = %d, want construction plus one top-level refresh", loader.callCount())
	}
	if len(inputs) != 2 {
		t.Fatalf("provider requests = %d, want initial and retry", len(inputs))
	}
	if !reflect.DeepEqual(inputs[0].Tools, inputs[1].Tools) {
		t.Fatal("skill tool schema changed across retry")
	}
	// Message 0 is the base context; the skill listing follows it.
	firstListing := llmUserText(t, inputs[0].Messages[1])
	secondListing := llmUserText(t, inputs[1].Messages[1])
	if firstListing != secondListing ||
		!strings.Contains(firstListing, `kind="skill_listing"`) {
		t.Fatalf("skill listing changed across retry:\nfirst:\n%s\nsecond:\n%s", firstListing, secondListing)
	}
	if got := executeSkill(t, session, "review"); !strings.Contains(got, "old body") {
		t.Fatalf("retry refreshed tool body unexpectedly: %q", got)
	}
}

func TestSkillRefreshPersistenceFailureDoesNotPublishRegistry(t *testing.T) {
	ctx := context.Background()
	loader := &mutableSkills{}
	loader.set(skills.Skill{
		Name: "review", Description: "review", Content: "old body", Dir: "/skills/review",
	})
	store := &checkpointStore{}
	modelCalls := 0
	session, err := New(ctx, Options{
		Model:       llm.Model{Provider: "test", ID: "model"},
		Tools:       []tools.Tool{},
		Store:       store,
		SkillLoader: loader.load,
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			modelCalls++
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(ctx, "first"); err != nil {
		t.Fatal(err)
	}

	loader.set(skills.Skill{
		Name: "review", Description: "review", Content: "new body", Dir: "/skills/review",
	})
	storeErr := errors.New("skill checkpoint failed")
	store.mu.Lock()
	store.failErr = storeErr
	store.mu.Unlock()

	err = session.Prompt(ctx, "second")
	if !errors.Is(err, storeErr) {
		t.Fatalf("Prompt error = %v, want persistence failure", err)
	}
	if modelCalls != 1 {
		t.Fatalf("model calls = %d, want no request after failed refresh checkpoint", modelCalls)
	}
	if got := executeSkill(t, session, "review"); !strings.Contains(got, "old body") {
		t.Fatalf("failed refresh published new registry: %q", got)
	}
	state := session.modelContext.State()
	if state.StagedSkillsRevision == "" || state.ActiveSkillsRevision != "" {
		t.Fatalf("model context state = %#v, want uncommitted staged update", state)
	}
}

func executeSkill(t *testing.T, session *Session, name string) string {
	t.Helper()
	result, err := executeSkillResult(session, name)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, content := range result.Content {
		if block, ok := content.(*llm.TextContent); ok {
			text += block.Text
		}
	}
	return text
}

func executeSkillResult(session *Session, name string) (agent.ToolResult, error) {
	tool := session.toolByName[skills.ToolName]
	raw, _ := json.Marshal(map[string]string{"name": name})
	return tool.Execute(context.Background(), "test-call", raw, nil)
}
