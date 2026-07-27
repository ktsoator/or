package provider

import (
	"testing"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all"
)

func TestIsUtilityModelEligible(t *testing.T) {
	tests := []struct {
		name  string
		model llm.Model
		want  bool
	}{
		{
			name: "plain text model",
			model: llm.Model{
				Protocol: llm.ProtocolOpenAICompletions,
				Input:    []llm.ModelInput{llm.Text},
			},
			want: true,
		},
		{
			name: "reasoning model with optional thinking",
			model: llm.Model{
				Protocol:  llm.ProtocolOpenAICompletions,
				Reasoning: true,
				Input:     []llm.ModelInput{llm.Text},
			},
			want: true,
		},
		{
			name: "reasoning model that requires thinking",
			model: llm.Model{
				Protocol:  llm.ProtocolOpenAICompletions,
				Reasoning: true,
				ThinkingLevelMap: map[llm.ModelThinkingLevel]*string{
					llm.ModelThinkingOff: nil,
				},
				Input: []llm.ModelInput{llm.Text},
			},
			want: false,
		},
		{
			name: "image-only model",
			model: llm.Model{
				Protocol: llm.ProtocolOpenAICompletions,
				Input:    []llm.ModelInput{llm.Image},
			},
			want: false,
		},
		{
			name: "unsupported protocol",
			model: llm.Model{
				Protocol: "unsupported",
				Input:    []llm.ModelInput{llm.Text},
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsUtilityModelEligible(test.model); got != test.want {
				t.Fatalf("IsUtilityModelEligible() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOpenCodeReasoningModelCanBeUsedWithThinkingOff(t *testing.T) {
	model, ok := llm.LookupModel("opencode-go", "deepseek-v4-flash")
	if !ok {
		t.Fatal("OpenCode Go DeepSeek V4 Flash is missing from the catalog")
	}
	if !IsUtilityModelEligible(model) {
		t.Fatal("OpenCode Go DeepSeek V4 Flash should be eligible when thinking is disabled")
	}
}
