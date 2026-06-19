package llm

import "testing"

func TestModelRegistryClonesRequiresThinkingAsText(t *testing.T) {
	requiresThinkingAsText := true
	registry := NewModelRegistry()
	err := registry.Register(Model{
		Provider: "test-provider",
		ID:       "test-model",
		Protocol: ProtocolOpenAICompletions,
		Compatibility: &OpenAICompletionsCompatibility{
			RequiresThinkingAsText: &requiresThinkingAsText,
		},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Mutating the source after registration must not alter the stored model.
	requiresThinkingAsText = false
	first, ok := registry.Get("test-provider", "test-model")
	if !ok {
		t.Fatal("Get() model not found")
	}
	firstCompatibility := first.Compatibility.(*OpenAICompletionsCompatibility)
	if firstCompatibility.RequiresThinkingAsText == nil || !*firstCompatibility.RequiresThinkingAsText {
		t.Fatalf("stored RequiresThinkingAsText = %v, want true", firstCompatibility.RequiresThinkingAsText)
	}

	// Mutating a returned model must not leak back into the registry.
	*firstCompatibility.RequiresThinkingAsText = false
	second, ok := registry.Get("test-provider", "test-model")
	if !ok {
		t.Fatal("Get() model not found on second lookup")
	}
	secondCompatibility := second.Compatibility.(*OpenAICompletionsCompatibility)
	if secondCompatibility.RequiresThinkingAsText == nil || !*secondCompatibility.RequiresThinkingAsText {
		t.Fatalf("second RequiresThinkingAsText = %v, want true", secondCompatibility.RequiresThinkingAsText)
	}
}
