package main

import (
	"context"
	"net/http"
)

const modelsDevURL = "https://models.dev/api.json"

func loadModelsDev(ctx context.Context, client *http.Client) (map[string]sourceProvider, error) {
	var catalog map[string]sourceProvider
	if err := getJSON(ctx, client, modelsDevURL, &catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}
