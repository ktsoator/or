package llm

import (
	"context"
	"testing"
)

func TestClientCompleteWithFakeProvider(t *testing.T) {
	registry := NewRegistry()

	provider := NewFakeProvider("hello from fake provider")
	if err := registry.Register(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	client := NewClient(registry)

	model := Model{
		ID:       "fake-1",
		Name:     "Fake Model",
		API:      "fake",
		Provider: "fake",
	}

	input := Context{
		Messages: []Message{
			{
				Role: RoleUser,
				Content: []Content{
					{
						Type: ContentText,
						Text: "hello",
					},
				},
			},
		},
	}

	message, err := client.Complete(
		context.Background(),
		model,
		input,
		StreamOptions{},
	)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if message.StopReason != "stop" {
		t.Fatalf(
			"expected stop reason %q, got %q",
			"stop",
			message.StopReason,
		)
	}

	if len(message.Content) != 1 {
		t.Fatalf(
			"expected one content block, got %d",
			len(message.Content),
		)
	}

	if message.Content[0].Text != "hello from fake provider" {
		t.Fatalf(
			"unexpected response: %q",
			message.Content[0].Text,
		)
	}
}
