package main

import (
	"context"
	"encoding/json"
	"net/http"
)

const vercelURL = "https://ai-gateway.vercel.sh/v1/models"

func fromVercel(ctx context.Context, client *http.Client) ([]model, error) {
	var raw struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := getJSON(ctx, client, vercelURL, &raw); err != nil {
		return nil, err
	}
	var models []model
	for _, item := range raw.Data {
		var id, name string
		var contextWindow, maxTokens int64
		var tags []string
		var pricing map[string]any
		_ = json.Unmarshal(item["id"], &id)
		_ = json.Unmarshal(item["name"], &name)
		_ = json.Unmarshal(item["context_window"], &contextWindow)
		_ = json.Unmarshal(item["max_tokens"], &maxTokens)
		_ = json.Unmarshal(item["tags"], &tags)
		_ = json.Unmarshal(item["pricing"], &pricing)
		if id == "" || !contains(tags, "tool-use") {
			continue
		}
		models = append(models, model{
			ID: id, Name: defaultString(name, id), Protocol: "anthropic-messages", Provider: "vercel-ai-gateway",
			BaseURL: "https://ai-gateway.vercel.sh", Reasoning: contains(tags, "reasoning"),
			Input: inputModalities(tags), InputCost: anyPerMillion(pricing["input"]), OutputCost: anyPerMillion(pricing["output"]),
			CacheReadCost: anyPerMillion(pricing["input_cache_read"]), CacheWriteCost: anyPerMillion(pricing["input_cache_write"]),
			ContextWindow: defaultInt(contextWindow, 4096), MaxTokens: defaultInt(maxTokens, 4096),
		})
	}
	return models, nil
}
