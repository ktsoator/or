package main

import (
	"context"
	"net/http"
)

const openRouterURL = "https://openrouter.ai/api/v1/models"

type openRouterResponse struct {
	Data []struct {
		ID                  string   `json:"id"`
		Name                string   `json:"name"`
		ContextLength       int64    `json:"context_length"`
		SupportedParameters []string `json:"supported_parameters"`
		Architecture        struct {
			Modality string `json:"modality"`
		} `json:"architecture"`
		Pricing struct {
			Prompt          string `json:"prompt"`
			Completion      string `json:"completion"`
			InputCacheRead  string `json:"input_cache_read"`
			InputCacheWrite string `json:"input_cache_write"`
		} `json:"pricing"`
		TopProvider struct {
			MaxCompletionTokens int64 `json:"max_completion_tokens"`
		} `json:"top_provider"`
	} `json:"data"`
}

func fromOpenRouter(ctx context.Context, client *http.Client) ([]model, error) {
	var response openRouterResponse
	if err := getJSON(ctx, client, openRouterURL, &response); err != nil {
		return nil, err
	}
	var models []model
	for _, source := range response.Data {
		if !contains(source.SupportedParameters, "tools") {
			continue
		}
		models = append(models, model{
			ID: source.ID, Name: defaultString(source.Name, source.ID), Protocol: "openai-completions",
			Provider: "openrouter", BaseURL: "https://openrouter.ai/api/v1",
			Reasoning: contains(source.SupportedParameters, "reasoning"),
			Input:     inputModalities([]string{source.Architecture.Modality}),
			InputCost: perMillion(source.Pricing.Prompt), OutputCost: perMillion(source.Pricing.Completion),
			CacheReadCost: perMillion(source.Pricing.InputCacheRead), CacheWriteCost: perMillion(source.Pricing.InputCacheWrite),
			ContextWindow: defaultInt(source.ContextLength, 4096), MaxTokens: defaultInt(source.TopProvider.MaxCompletionTokens, 4096),
		})
	}
	return models, nil
}
