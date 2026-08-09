package conversation

import (
	"context"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestGenerateAITitleUsesSessionModelWithThinkingDisabled(t *testing.T) {
	model := llm.Model{
		ID:       "session-model",
		Provider: "test-provider",
		Protocol: llm.ProtocolOpenAICompletions,
	}
	title, err := generateAITitleWith(
		context.Background(),
		model,
		"Titles are not generated",
		func(
			_ context.Context,
			gotModel llm.Model,
			_ llm.Context,
			options llm.StreamOptions,
		) (llm.AssistantMessage, error) {
			if gotModel.Provider != model.Provider || gotModel.ID != model.ID {
				t.Fatalf("model = %s/%s, want %s/%s", gotModel.Provider, gotModel.ID, model.Provider, model.ID)
			}
			if options.Reasoning != llm.ModelThinkingOff {
				t.Fatalf("reasoning = %q, want off", options.Reasoning)
			}
			if options.MaxTokens != titleMaxOutputTokens {
				t.Fatalf("max tokens = %d, want %d", options.MaxTokens, titleMaxOutputTokens)
			}
			return llm.AssistantMessage{
				Content: []llm.AssistantContent{
					&llm.TextContent{Text: `{"title":"Fix title generation"}`},
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Fix title generation" {
		t.Fatalf("title = %q", title)
	}
}

func TestParseAITitleAcceptsWrappedJSONAndPlainText(t *testing.T) {
	for input, want := range map[string]string{
		"result: {\"title\":\"Fix mobile login\"}": "Fix mobile login",
		"'Refactor provider settings'":             "Refactor provider settings",
	} {
		if got := parseAITitle(input); got != want {
			t.Errorf("parseAITitle(%q) = %q, want %q", input, got, want)
		}
	}
}
