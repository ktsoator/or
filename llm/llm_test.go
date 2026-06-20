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
