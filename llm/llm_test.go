package llm_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestDefaultClientIncludesBuiltInAdapters(t *testing.T) {
	tests := []struct {
		name     string
		protocol llm.Protocol
		want     string
	}{
		{name: "openai", protocol: llm.ProtocolOpenAICompletions, want: "OpenAI API key is empty"},
		{name: "anthropic", protocol: llm.ProtocolAnthropicMessages, want: "Anthropic API key is empty"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := llm.Stream(context.Background(), llm.Model{
				ID:       "test-model",
				Protocol: test.protocol,
				Provider: "facade-test",
			}, llm.Context{}, llm.StreamOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Stream() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDefaultClientRejectsUnknownProtocol(t *testing.T) {
	_, err := llm.Stream(context.Background(), llm.Model{
		ID:       "test-model",
		Protocol: "unknown",
		Provider: "facade-test",
	}, llm.Context{}, llm.StreamOptions{})
	if err == nil || !strings.Contains(err.Error(), "no adapter registered") {
		t.Fatalf("Stream() error = %v", err)
	}
}

func TestDefaultClientRejectsMismatchedProtocolOptions(t *testing.T) {
	_, err := llm.Stream(context.Background(), llm.Model{
		ID:       "test-model",
		Protocol: llm.ProtocolOpenAICompletions,
		Provider: "facade-test",
	}, llm.Context{}, llm.StreamOptions{
		ProtocolOptions: &llm.AnthropicStreamOptions{
			ThinkingDisplay: llm.ThinkingDisplayOmitted,
		},
	})
	if err == nil || !strings.Contains(err.Error(), string(llm.ProtocolAnthropicMessages)) {
		t.Fatalf("Stream() error = %v, want protocol options mismatch", err)
	}
}

func TestDefaultClientAcceptsPublicOpenAIToolChoice(t *testing.T) {
	_, err := llm.Stream(context.Background(), llm.Model{
		ID:       "test-model",
		Protocol: llm.ProtocolOpenAICompletions,
		Provider: "facade-test",
	}, llm.Context{
		Tools: []llm.ToolDefinition{{Name: "weather"}},
	}, llm.StreamOptions{
		ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
			ToolChoice: llm.OpenAIToolChoiceRequired,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "OpenAI API key is empty") {
		t.Fatalf("Stream() error = %v, want validation to reach OpenAI adapter", err)
	}
}

func TestGetModelUsesBuiltInCatalog(t *testing.T) {
	model := llm.GetModel("deepseek", "deepseek-chat")
	if model.Provider != "deepseek" || model.Protocol != llm.ProtocolOpenAICompletions {
		t.Fatalf("model = %#v", model)
	}
}

// echoAdapter is a minimal custom ProtocolAdapter built only from the public
// surface: it emits one text block through NewStreamWriter and finishes.
type echoAdapter struct{}

func (echoAdapter) Protocol() llm.Protocol { return "echo" }

func (echoAdapter) Stream(
	ctx context.Context, model llm.Model, _ llm.Context, _ llm.StreamOptions,
) (<-chan llm.Event, error) {
	events := make(chan llm.Event)
	go func() {
		defer close(events)
		message := llm.AssistantMessage{Protocol: model.Protocol, Provider: model.Provider, Model: model.ID}
		writer := llm.NewStreamWriter(ctx, events, &message)
		text := &llm.TextContent{}
		message.Content = append(message.Content, text)
		writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: 0})
		text.Text = "echo"
		writer.Emit(llm.Event{Type: llm.EventTextDelta, ContentIndex: 0, Delta: "echo"})
		writer.Emit(llm.Event{Type: llm.EventTextEnd, ContentIndex: 0, Content: "echo"})
		message.StopReason = llm.StopReasonStop
		writer.Done()
	}()
	return events, nil
}

// A custom protocol adapter registered through the public registry serves
// alongside the built-ins, and NewStreamWriter produces a well-formed stream.
func TestCustomProtocolAdapterViaRegistry(t *testing.T) {
	registry := llm.NewRegistry()
	llm.RegisterBuiltins(registry)
	if err := registry.Register(echoAdapter{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	client := llm.NewClientWithRegistry(registry)

	message, err := client.Complete(context.Background(), llm.Model{
		ID: "echo-1", Provider: "echo", Protocol: "echo",
	}, llm.Context{Messages: []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hi"}}},
	}}, llm.StreamOptions{})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if message.StopReason != llm.StopReasonStop || len(message.Content) != 1 {
		t.Fatalf("message = %#v", message)
	}
	text, ok := message.Content[0].(*llm.TextContent)
	if !ok || text.Text != "echo" {
		t.Fatalf("content = %#v", message.Content[0])
	}

	// The built-ins remain registered on the same client.
	if _, err := client.Stream(context.Background(), llm.Model{
		ID: "m", Protocol: llm.ProtocolAnthropicMessages, Provider: "echo-test",
	}, llm.Context{}, llm.StreamOptions{}); err == nil ||
		!strings.Contains(err.Error(), "Anthropic API key is empty") {
		t.Fatalf("built-in adapter missing from custom registry: %v", err)
	}
}
