package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

func collect(ctx context.Context, client *http.Client) ([]model, error) {
	catalog, err := loadModelsDev(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("models.dev: %w", err)
	}

	models := fromModelsDev(catalog)
	if openRouter, err := fromOpenRouter(ctx, client); err != nil {
		fmt.Fprintf(os.Stderr, "warning: OpenRouter catalog unavailable: %v\n", err)
	} else {
		models = append(models, openRouter...)
	}
	if vercel, err := fromVercel(ctx, client); err != nil {
		fmt.Fprintf(os.Stderr, "warning: Vercel AI Gateway catalog unavailable: %v\n", err)
	} else {
		models = append(models, vercel...)
	}

	applyOverrides(models)
	return deduplicate(models), nil
}
