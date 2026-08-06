package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/skills"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestExplicitSkillReachesProviderWithoutChangingInstructions(t *testing.T) {
	var captured llm.Context
	var userEvent Event
	store := &checkpointStore{}
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Store: store,
		Skills: []skills.Skill{{
			Name:        "deploy",
			Description: "Deploy the application",
			Content:     "Deploy target from the user request. Keep $ARGUMENTS and $1 literal.",
			Dir:         "/skills/deploy",
			Path:        "/skills/deploy/SKILL.md",
		}},
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
	session.Subscribe(func(event Event) {
		if event.Type == UserMessageCompleted {
			userEvent = event
		}
	})

	if err := session.Prompt(context.Background(), "$deploy staging"); err != nil {
		t.Fatal(err)
	}
	if len(captured.Messages) < 2 {
		t.Fatalf("provider messages = %d, want context and explicit user message", len(captured.Messages))
	}
	user, ok := captured.Messages[len(captured.Messages)-1].(*llm.UserMessage)
	if !ok {
		t.Fatalf("provider message = %T, want user", captured.Messages[len(captured.Messages)-1])
	}
	if len(user.Content) != 2 {
		t.Fatalf("user content blocks = %d, want visible mention and loaded instructions", len(user.Content))
	}
	visible, _ := user.Content[0].(*llm.TextContent)
	loaded, _ := user.Content[1].(*llm.TextContent)
	if visible == nil || visible.Text != "[$deploy](/skills/deploy/SKILL.md) staging" {
		t.Fatalf("visible block = %#v", visible)
	}
	if loaded == nil ||
		!strings.Contains(loaded.Text, "<agent-skill-invocation") ||
		!strings.Contains(loaded.Text, `task details remain in the visible message: "staging"`) ||
		!strings.Contains(loaded.Text, "Keep $ARGUMENTS and $1 literal.") {
		t.Fatalf("loaded block = %#v", loaded)
	}
	var contextText string
	for _, message := range captured.Messages[:len(captured.Messages)-1] {
		contextText += llmUserText(t, message)
	}
	if !strings.Contains(contextText, "deploy") {
		t.Fatal("skill missing from the model-visible skill listing")
	}

	history := session.History()
	if len(history) == 0 || history[0].Type != HistoryUser ||
		history[0].Text != "[$deploy](/skills/deploy/SKILL.md) staging" {
		t.Fatalf("history = %#v, want visible skill reference", history)
	}
	if userEvent.Text != "[$deploy](/skills/deploy/SKILL.md) staging" {
		t.Fatalf("user event text = %q", userEvent.Text)
	}
	result, err := executeSkillResult(session, "deploy")
	if err != nil || !strings.Contains(eventToolResultText(result), "Keep $ARGUMENTS and $1 literal.") {
		t.Fatalf("skill tool result = %#v error = %v", result, err)
	}
}

func TestUnknownExplicitSkillDoesNotReachProvider(t *testing.T) {
	requests := 0
	session, err := New(context.Background(), Options{
		Model:  llm.Model{Provider: "test", ID: "model"},
		Tools:  []tools.Tool{},
		Skills: []skills.Skill{{Name: "review", Description: "review changes", Content: "review"}},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			requests++
			return assistantEvents(model, "done"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = session.Prompt(context.Background(), "$missing args")
	if err == nil || !strings.Contains(err.Error(), `Unknown skill "missing"`) {
		t.Fatalf("error = %v, want explicit unknown skill error", err)
	}
	if requests != 0 {
		t.Fatalf("provider requests = %d, want zero", requests)
	}
}

func TestUserAuthoredInvocationWrapperPrefixRemainsVisible(t *testing.T) {
	const authored = `<agent-skill-invocation name="example">user text`
	text, _, _ := userMessageContent(&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: authored}},
	})
	if text != authored {
		t.Fatalf("user-authored text = %q, want %q", text, authored)
	}
}
