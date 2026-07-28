package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const minimumCompleteCatalogModels = 300

var requiredCompleteCatalogProviders = []string{
	"anthropic",
	"deepseek",
	"opencode",
}

type collectOptions struct {
	AllowPartial bool
	Warnings     io.Writer
	sources      []catalogSource
}

type catalogSource struct {
	name string
	load func(context.Context, *http.Client) ([]model, error)
}

func collect(ctx context.Context, client *http.Client, options collectOptions) ([]model, error) {
	sources := options.sources
	if len(sources) == 0 {
		sources = defaultCatalogSources()
	}

	var models []model
	var failures []error
	for _, source := range sources {
		loaded, err := source.load(ctx, client)
		if err == nil && len(loaded) == 0 {
			err = errors.New("returned no usable models")
		}
		if err != nil {
			failure := fmt.Errorf("%s: %w", source.name, err)
			failures = append(failures, failure)
			if options.AllowPartial && options.Warnings != nil {
				fmt.Fprintf(options.Warnings, "warning: catalog source unavailable: %v\n", failure)
			}
			continue
		}
		models = append(models, loaded...)
	}

	if len(failures) > 0 && !options.AllowPartial {
		return nil, fmt.Errorf("catalog source failure: %w", errors.Join(failures...))
	}
	if len(models) == 0 {
		if len(failures) > 0 {
			return nil, fmt.Errorf("no catalog source succeeded: %w", errors.Join(failures...))
		}
		return nil, errors.New("no catalog source produced models")
	}

	applyOverrides(models)
	models = deduplicate(models)
	if err := validateCatalog(models, options.AllowPartial); err != nil {
		return nil, err
	}
	return models, nil
}

func defaultCatalogSources() []catalogSource {
	return []catalogSource{
		{name: "models.dev", load: loadModelsDevModels},
	}
}

func loadModelsDevModels(ctx context.Context, client *http.Client) ([]model, error) {
	catalog, err := loadModelsDev(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := validateProviderRules(catalog); err != nil {
		return nil, fmt.Errorf("validate models.dev provider rules: %w", err)
	}
	return fromModelsDev(catalog), nil
}

func validateCatalog(models []model, allowPartial bool) error {
	if len(models) == 0 {
		return errors.New("generated catalog is empty")
	}
	if err := validateThinkingProfiles(openCodeThinkingProfiles); err != nil {
		return fmt.Errorf("invalid OpenCode thinking profiles: %w", err)
	}

	providers := make(map[string]struct{})
	seen := make(map[string]struct{}, len(models))
	for index, candidate := range models {
		if candidate.Provider == "" || candidate.ID == "" || candidate.Protocol == "" {
			return fmt.Errorf("generated catalog model %d has incomplete identity or routing", index)
		}
		if candidate.ContextWindow <= 0 || candidate.MaxTokens <= 0 {
			return fmt.Errorf(
				"generated catalog model %s/%s has invalid token limits",
				candidate.Provider,
				candidate.ID,
			)
		}
		if profile, ok := openCodeThinkingProfiles[modelRouteKey{Provider: candidate.Provider, ModelID: candidate.ID}]; ok {
			if err := validateAppliedThinkingProfile(candidate, profile); err != nil {
				return err
			}
		}
		key := candidate.Provider + "\x00" + candidate.ID
		if _, exists := seen[key]; exists {
			return fmt.Errorf("generated catalog contains duplicate model %s/%s", candidate.Provider, candidate.ID)
		}
		seen[key] = struct{}{}
		providers[candidate.Provider] = struct{}{}
	}

	if allowPartial {
		return nil
	}
	if len(models) < minimumCompleteCatalogModels {
		return fmt.Errorf(
			"generated catalog has %d models; complete catalogs require at least %d",
			len(models),
			minimumCompleteCatalogModels,
		)
	}
	for _, provider := range requiredCompleteCatalogProviders {
		if _, exists := providers[provider]; !exists {
			return fmt.Errorf("generated catalog is missing required provider %q", provider)
		}
	}
	return nil
}
