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
			name: "explicit Xiaomi format is not direct OpenAI effort",
			model: llm.Model{
				Protocol: llm.ProtocolOpenAICompletions,
				Compatibility: &llm.OpenAICompletionsCompatibility{
					ThinkingFormat: string(ThinkingFormatXiaomi),
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

func TestDetectXiaomiThinkingFormat(t *testing.T) {
	tests := []struct {
		name  string
		model llm.Model
		want  ThinkingFormat
	}{
		{
			name:  "native provider",
			model: llm.Model{Provider: "xiaomi", BaseURL: "https://proxy.example/v1"},
			want:  ThinkingFormatXiaomi,
		},
		{
			name:  "token plan provider",
			model: llm.Model{Provider: "xiaomi-token-plan-cn"},
			want:  ThinkingFormatXiaomi,
		},
		{
			name:  "official API hostname",
			model: llm.Model{Provider: "custom", BaseURL: "https://api.xiaomimimo.com/v1"},
			want:  ThinkingFormatXiaomi,
		},
		{
			name:  "official token plan hostname",
			model: llm.Model{Provider: "custom", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1"},
			want:  ThinkingFormatXiaomi,
		},
		{
			name:  "OpenCode MiMo route stays isolated",
			model: llm.Model{Provider: "opencode-go", ID: "mimo-v2.5", BaseURL: "https://opencode.ai/zen/go/v1"},
			want:  ThinkingFormatOpenAI,
		},
		{
			name:  "lookalike hostname is rejected",
			model: llm.Model{Provider: "custom", ID: "mimo-v2.5", BaseURL: "https://api.xiaomimimo.com.example/v1"},
			want:  ThinkingFormatOpenAI,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Detect(test.model)
			if got.ThinkingFormat != test.want {
				t.Fatalf("ThinkingFormat = %q, want %q", got.ThinkingFormat, test.want)
			}
			if test.want == ThinkingFormatXiaomi {
				if got.SupportsReasoningEffort {
					t.Fatal("Xiaomi must not support OpenAI reasoning_effort")
				}
				if !got.RequiresReasoningContentOnAssistantMessages {
					t.Fatal("Xiaomi must replay assistant reasoning_content")
				}
			}
		})
	}
}
