package llm

import (
	"strings"
	"testing"
)

func TestStreamOptionsValidateAcceptsMatchingAnthropicExtension(t *testing.T) {
	options := StreamOptions{
		ProtocolOptions: &AnthropicStreamOptions{ThinkingDisplay: ThinkingDisplayOmitted},
	}
	if err := options.Validate(ProtocolAnthropicMessages, nil); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestStreamOptionsValidateRejectsAnthropicExtensionForOtherProtocol(t *testing.T) {
	options := StreamOptions{ProtocolOptions: &AnthropicStreamOptions{}}
	err := options.Validate(ProtocolOpenAICompletions, nil)
	if err == nil || !strings.Contains(err.Error(), string(ProtocolAnthropicMessages)) {
		t.Fatalf("Validate() error = %v, want Anthropic protocol mismatch", err)
	}
}

func TestStreamOptionsValidateRejectsUnknownThinkingDisplay(t *testing.T) {
	options := StreamOptions{
		ProtocolOptions: &AnthropicStreamOptions{ThinkingDisplay: ThinkingDisplay("verbatim")},
	}
	err := options.Validate(ProtocolAnthropicMessages, nil)
	if err == nil || !strings.Contains(err.Error(), "thinking display") {
		t.Fatalf("Validate() error = %v, want unsupported display", err)
	}
}

func TestStreamOptionsValidateRejectsTypedNilProtocolOptions(t *testing.T) {
	var anthropicOptions *AnthropicStreamOptions
	options := StreamOptions{ProtocolOptions: anthropicOptions}
	err := options.Validate(ProtocolAnthropicMessages, nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("Validate() error = %v, want typed nil error", err)
	}
}

func TestStreamOptionsValidateAcceptsNativeToolChoices(t *testing.T) {
	tools := []ToolDefinition{{Name: "weather"}}
	tests := []struct {
		name     string
		protocol Protocol
		options  ProtocolStreamOptions
	}{
		{
			name:     "Anthropic any",
			protocol: ProtocolAnthropicMessages,
			options:  &AnthropicStreamOptions{ToolChoice: AnthropicToolChoiceAny},
		},
		{
			name:     "Anthropic named",
			protocol: ProtocolAnthropicMessages,
			options:  &AnthropicStreamOptions{ToolChoice: AnthropicToolChoiceTool{Name: "weather"}},
		},
		{
			name:     "OpenAI required",
			protocol: ProtocolOpenAICompletions,
			options:  &OpenAICompletionsStreamOptions{ToolChoice: OpenAIToolChoiceRequired},
		},
		{
			name:     "OpenAI named",
			protocol: ProtocolOpenAICompletions,
			options:  &OpenAICompletionsStreamOptions{ToolChoice: OpenAIToolChoiceFunction{Name: "weather"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := StreamOptions{ProtocolOptions: test.options}
			if err := options.Validate(test.protocol, tools); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestStreamOptionsValidateRejectsInvalidNativeToolChoices(t *testing.T) {
	tools := []ToolDefinition{{Name: "weather"}}
	tests := []struct {
		name     string
		protocol Protocol
		options  ProtocolStreamOptions
		tools    []ToolDefinition
		want     string
	}{
		{
			name:     "choice without tools",
			protocol: ProtocolOpenAICompletions,
			options:  &OpenAICompletionsStreamOptions{ToolChoice: OpenAIToolChoiceRequired},
			want:     "at least one tool",
		},
		{
			name:     "unknown mode",
			protocol: ProtocolAnthropicMessages,
			options:  &AnthropicStreamOptions{ToolChoice: AnthropicToolChoiceMode("required")},
			tools:    tools,
			want:     "unsupported Anthropic tool choice",
		},
		{
			name:     "unknown named tool",
			protocol: ProtocolOpenAICompletions,
			options:  &OpenAICompletionsStreamOptions{ToolChoice: OpenAIToolChoiceFunction{Name: "missing"}},
			tools:    tools,
			want:     "unknown tool",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := StreamOptions{ProtocolOptions: test.options}
			err := options.Validate(test.protocol, test.tools)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}
