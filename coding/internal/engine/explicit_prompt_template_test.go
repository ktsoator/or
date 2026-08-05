package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/coding/internal/prompttemplate"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestPromptTemplateExpandsForProviderButNotHistoryOrEvents(t *testing.T) {
	var captured llm.Context
	var userEvent Event
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		PromptTemplates: []prompttemplate.Template{{
			Name: "review", Content: "Review $1 and ${@:2}", Path: "/prompts/review.md",
			Source: prompttemplate.SourceProject,
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

	if err := session.Prompt(context.Background(), `/review security "error paths"`); err != nil {
		t.Fatal(err)
	}
	user, ok := captured.Messages[1].(*llm.UserMessage)
	if !ok || len(user.Content) != 2 {
		t.Fatalf("provider user message = %#v", captured.Messages[1])
	}
	expanded, _ := user.Content[1].(*llm.TextContent)
	if expanded == nil || !strings.Contains(expanded.Text, "Review security and error paths") {
		t.Fatalf("expanded block = %#v", expanded)
	}
	const visible = `/review security "error paths"`
	history := session.History()
	if len(history) == 0 || history[0].Text != visible {
		t.Fatalf("history = %#v, want visible invocation", history)
	}
	if history[0].Invocation == nil || history[0].Invocation.Name != "review" ||
		history[0].Invocation.Source != "project" ||
		history[0].Invocation.Path != "/prompts/review.md" {
		t.Fatalf("history invocation = %#v", history[0].Invocation)
	}
	if userEvent.Text != visible {
		t.Fatalf("event text = %q, want %q", userEvent.Text, visible)
	}
	if userEvent.Invocation == nil || userEvent.Invocation.Name != "review" ||
		userEvent.Invocation.Source != "project" {
		t.Fatalf("event invocation = %#v", userEvent.Invocation)
	}
}

func TestUnknownSlashMessageHasNoPromptTemplateInvocation(t *testing.T) {
	var userEvent Event
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		PromptTemplates: []prompttemplate.Template{{
			Name: "review", Content: "Review changes", Path: "/prompts/review.md",
			Source: prompttemplate.SourceProject,
		}},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
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

	if err := session.Prompt(context.Background(), "/hello"); err != nil {
		t.Fatal(err)
	}
	if userEvent.Text != "/hello" || userEvent.Invocation != nil {
		t.Fatalf("user event = %#v", userEvent)
	}
	history := session.History()
	if len(history) == 0 || history[0].Text != "/hello" || history[0].Invocation != nil {
		t.Fatalf("history = %#v", history)
	}
}

func TestPromptTemplateLoaderRefreshesBeforeInvocation(t *testing.T) {
	content := "first"
	var captured llm.Context
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		PromptTemplateLoader: func() []prompttemplate.Template {
			return []prompttemplate.Template{{Name: "review", Content: content}}
		},
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
	content = "updated"
	if err := session.Prompt(context.Background(), "/review"); err != nil {
		t.Fatal(err)
	}
	user := captured.Messages[1].(*llm.UserMessage)
	expanded := user.Content[1].(*llm.TextContent)
	if !strings.Contains(expanded.Text, "updated") {
		t.Fatalf("expanded = %q, want refreshed content", expanded.Text)
	}
}
