package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

func deduplicate(models []model) []model {
	seen := make(map[string]model, len(models))
	for _, m := range models {
		if m.ID == "" || m.Provider == "" || m.Protocol == "" {
			continue
		}
		key := m.Provider + "\x00" + m.ID
		if _, exists := seen[key]; !exists {
			seen[key] = m
		}
	}
	result := make([]model, 0, len(seen))
	for _, m := range seen {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func render(models []model) ([]byte, error) {
	// models arrives deduplicated and sorted by provider then ID, so the flat
	// catalog stays grouped and stable without an intermediate map.
	catalog := make([]catalogModel, 0, len(models))
	for _, source := range models {
		catalog = append(catalog, toCatalogModel(source))
	}
	encoded, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated catalog: %w", err)
	}
	return append(encoded, '\n'), nil
}

func toCatalogModel(source model) catalogModel {
	var compat *compatibility
	if source.Compat.Kind != "" {
		value := source.Compat
		compat = &value
	}
	return catalogModel{
		ID:                 source.ID,
		Name:               source.Name,
		Provider:           source.Provider,
		Protocol:           source.Protocol,
		BaseURL:            source.BaseURL,
		Reasoning:          source.Reasoning,
		ThinkingLevelMap:   source.ThinkingLevelMap,
		ThinkingVisibility: source.ThinkingVisibility,
		Input:              source.Input,
		Cost: catalogCost{
			Input:      source.InputCost,
			Output:     source.OutputCost,
			CacheRead:  source.CacheReadCost,
			CacheWrite: source.CacheWriteCost,
		},
		ContextWindow: source.ContextWindow,
		MaxTokens:     source.MaxTokens,
		Headers:       source.Headers,
		Compatibility: compat,
	}
}
