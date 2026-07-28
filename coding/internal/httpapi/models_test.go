package httpapi

import (
	"slices"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestNewModelOptionIncludesThinkingPresentation(t *testing.T) {
	option := newModelOption(llm.Model{
		Provider:  "opencode-go",
		ID:        "minimax-m2.7",
		Reasoning: true,
		ThinkingLevelMap: map[llm.ModelThinkingLevel]*string{
			llm.ModelThinkingOff:     nil,
			llm.ModelThinkingMinimal: nil,
			llm.ModelThinkingLow:     nil,
			llm.ModelThinkingMedium:  nil,
			llm.ModelThinkingXHigh:   nil,
			llm.ModelThinkingMax:     nil,
		},
		ThinkingVisibility: llm.ModelThinkingHidden,
		Input:              []llm.ModelInput{llm.Text, llm.Image},
		ContextWindow:      204_800,
	})

	if option.ThinkingVisibility != llm.ModelThinkingHidden {
		t.Fatalf("ThinkingVisibility = %q, want %q", option.ThinkingVisibility, llm.ModelThinkingHidden)
	}
	if !slices.Equal(option.ThinkingLevels, []llm.ModelThinkingLevel{llm.ModelThinkingHigh}) {
		t.Fatalf("ThinkingLevels = %v, want [high]", option.ThinkingLevels)
	}
	if !option.SupportsImages {
		t.Fatal("SupportsImages = false, want true")
	}
}

func TestNewModelOptionFallsBackToModelID(t *testing.T) {
	option := newModelOption(llm.Model{Provider: "demo", ID: "demo-model"})
	if option.Name != "demo-model" {
		t.Fatalf("Name = %q, want model ID", option.Name)
	}
}
