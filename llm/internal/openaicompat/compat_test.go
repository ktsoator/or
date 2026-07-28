package openaicompat

import (
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestSupportsDirectReasoningEffort(t *testing.T) {
	unsupported := false
	tests := []struct {
		name  string
		model llm.Model
		want  bool
	}{
		{
			name:  "Groq uses OpenAI effort",
			model: llm.Model{Protocol: llm.ProtocolOpenAICompletions, Provider: "groq"},
			want:  true,
		},
		{
			name:  "Cerebras uses OpenAI effort",
			model: llm.Model{Protocol: llm.ProtocolOpenAICompletions, Provider: "cerebras"},
			want:  true,
		},
		{
			name:  "xAI does not support effort",
			model: llm.Model{Protocol: llm.ProtocolOpenAICompletions, Provider: "xai"},
		},
		{
			name:  "DeepSeek uses a different thinking format",
			model: llm.Model{Protocol: llm.ProtocolOpenAICompletions, Provider: "deepseek"},
		},
		{
			name: "explicit format override wins",
			model: llm.Model{
				Protocol: llm.ProtocolOpenAICompletions,
				Compatibility: &llm.OpenAICompletionsCompatibility{
					ThinkingFormat: "qwen",
				},
			},
		},
		{
			name: "explicit effort opt-out wins",
			model: llm.Model{
				Protocol: llm.ProtocolOpenAICompletions,
				Compatibility: &llm.OpenAICompletionsCompatibility{
					SupportsReasoningEffort: &unsupported,
				},
			},
		},
		{
			name:  "Anthropic protocol is excluded",
			model: llm.Model{Protocol: llm.ProtocolAnthropicMessages, Provider: "anthropic"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SupportsDirectReasoningEffort(test.model); got != test.want {
				t.Fatalf("SupportsDirectReasoningEffort() = %v, want %v", got, test.want)
			}
		})
	}
}
