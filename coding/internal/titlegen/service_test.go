package titlegen

import (
	"context"
	"testing"

	"github.com/ktsoator/or/coding/internal/provider"
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all"
)

func TestGenerateUsesUtilityRouteWithThinkingDisabled(t *testing.T) {
	registry := llm.NewProviderRegistry()
	model := llm.Model{
		ID:       "small-model",
		Name:     "Small Model",
		Provider: "test-provider",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL:  "https://catalog.example.com/v1",
		Input:    []llm.ModelInput{llm.ModelInputText},
	}
	if err := registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
		ID:     "test-provider",
		Name:   "Test Provider",
		Models: []llm.Model{model},
	})); err != nil {
		t.Fatal(err)
	}
	store, err := provider.NewStore(t.TempDir(), registry)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Replace("test-provider", provider.Update{
		ActiveConnectionID: "utility",
		Connections: []provider.ConnectionUpdate{
			{ID: provider.OfficialConnectionID},
			{
				ID:          "utility",
				Name:        "Utility",
				BaseURL:     "https://utility.example.com/v1",
				ActiveKeyID: "title-key",
				Keys: []provider.KeyUpdate{{
					ID: "title-key", Name: "Title", APIKey: "title-secret",
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SetUtilityModel(provider.UtilityModelSelection{
		Provider:     "test-provider",
		Model:        "small-model",
		ConnectionID: "utility",
		KeyID:        "title-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := New(store)
	service.complete = func(
		_ context.Context,
		gotModel llm.Model,
		_ llm.Context,
		options llm.StreamOptions,
	) (llm.AssistantMessage, error) {
		if gotModel.ID != model.ID {
			t.Fatalf("model = %q", gotModel.ID)
		}
		if options.Reasoning != llm.ModelThinkingOff {
			t.Fatalf("reasoning = %q, want off", options.Reasoning)
		}
		if options.MaxTokens != maxOutputTokens {
			t.Fatalf("max tokens = %d", options.MaxTokens)
		}
		if options.BaseURL != "https://utility.example.com/v1" || options.APIKey != "title-secret" {
			t.Fatalf("request route = %#v", options)
		}
		return llm.AssistantMessage{
			Content: []llm.AssistantContent{&llm.TextContent{Text: `{"title":"Fix title generation"}`}},
		}, nil
	}

	result, err := service.Generate(context.Background(), "Titles are not generated")
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Fix title generation" || result.Provider != "test-provider" || result.Model != "small-model" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGenerateReportsUnavailableUtilityModel(t *testing.T) {
	store, err := provider.NewStore(t.TempDir(), llm.NewProviderRegistry())
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(store).Generate(context.Background(), "Hello")
	if ErrorCode(err) != CodeUnavailable {
		t.Fatalf("error = %v, code = %q", err, ErrorCode(err))
	}
}
