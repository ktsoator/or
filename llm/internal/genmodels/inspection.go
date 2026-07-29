package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

const (
	inspectionMarkerName    = ".or-genmodels-inspection"
	inspectionMarkerContent = "or-genmodels-inspection-v1\n"
	inspectionSchemaVersion = 1
)

type inspectionRouteSpec struct {
	Source   string
	Provider string
	Kind     string
	Protocol string
	BaseURL  string
}

type inspectionIndex struct {
	SchemaVersion    int                         `json:"schemaVersion"`
	SourceURL        string                      `json:"sourceUrl"`
	BeforeSemantics  string                      `json:"beforeSemantics"`
	ProviderCount    int                         `json:"providerCount"`
	SourceCount      int                         `json:"sourceCount"`
	SourceModelCount int                         `json:"sourceModelCount"`
	OutputModelCount int                         `json:"outputModelCount"`
	Providers        []inspectionProviderSummary `json:"providers"`
}

type inspectionProviderSummary struct {
	Provider       string `json:"provider"`
	Source         string `json:"source"`
	SourcePresent  bool   `json:"sourcePresent"`
	InputModels    int    `json:"inputModels"`
	OutputModels   int    `json:"outputModels"`
	FilteredModels int    `json:"filteredModels"`
}

type inspectionRoute struct {
	Provider        string   `json:"provider"`
	Source          string   `json:"source"`
	SourcePresent   bool     `json:"sourcePresent"`
	Kind            string   `json:"kind"`
	SharedWith      []string `json:"sharedWith,omitempty"`
	Protocol        string   `json:"protocol,omitempty"`
	BaseURL         string   `json:"baseUrl,omitempty"`
	OutputProtocols []string `json:"outputProtocols"`
	OutputBaseURLs  []string `json:"outputBaseUrls"`
}

type inspectionInput struct {
	Source string                 `json:"source"`
	Models map[string]sourceModel `json:"models"`
}

type providerInspection struct {
	Route  inspectionRoute
	Before inspectionInput
	After  []catalogModel
}

func generateInspection(
	ctx context.Context,
	client *http.Client,
	directory string,
	stdout io.Writer,
) error {
	snapshot, err := loadModelsDevSnapshot(ctx, client)
	if err != nil {
		return err
	}
	models, err := finalizeModels(snapshot.Models, false)
	if err != nil {
		return err
	}
	if err := writeProviderInspection(directory, snapshot.Catalog, models); err != nil {
		return fmt.Errorf("write inspection %s: %w", directory, err)
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "generated inspection %s with %d providers and %d models\n", directory, len(providerInspectionRouteSpecs()), len(models))
	}
	return nil
}

func providerInspectionRouteSpecs() []inspectionRouteSpec {
	routes := make([]inspectionRouteSpec, 0, len(providerRules)+len(openCodeVariants)+1)
	for _, rule := range providerRules {
		routes = append(routes, inspectionRouteSpec{
			Source:   rule.Source,
			Provider: rule.Provider,
			Kind:     "fixed",
			Protocol: rule.Protocol,
			BaseURL:  rule.BaseURL,
		})
	}
	for _, variant := range openCodeVariants {
		routes = append(routes, inspectionRouteSpec{
			Source:   variant.Source,
			Provider: variant.Provider,
			Kind:     "opencode",
		})
	}
	routes = append(routes, inspectionRouteSpec{
		Source:   githubCopilotSource,
		Provider: githubCopilotProvider,
		Kind:     "copilot",
	})
	sort.Slice(routes, func(i, j int) bool { return routes[i].Provider < routes[j].Provider })
	return routes
}

func writeProviderInspection(
	directory string,
	catalog map[string]sourceProvider,
	models []model,
) error {
	return writeProviderInspectionWithRoutes(directory, catalog, models, providerInspectionRouteSpecs())
}

func writeProviderInspectionWithRoutes(
	directory string,
	catalog map[string]sourceProvider,
	models []model,
	routes []inspectionRouteSpec,
) error {
	index, providers, err := buildProviderInspections(catalog, models, routes)
	if err != nil {
		return err
	}
	return writeInspectionDirectory(directory, func(root string) error {
		for _, provider := range providers {
			providerDirectory := filepath.Join(root, provider.Route.Provider)
			if err := os.Mkdir(providerDirectory, 0o755); err != nil {
				return err
			}
			if err := writeInspectionJSON(filepath.Join(providerDirectory, "route.json"), provider.Route); err != nil {
				return err
			}
			if err := writeInspectionJSON(filepath.Join(providerDirectory, "before.json"), provider.Before); err != nil {
				return err
			}
			if err := writeInspectionJSON(filepath.Join(providerDirectory, "after.json"), provider.After); err != nil {
				return err
			}
		}
		return writeInspectionJSON(filepath.Join(root, "index.json"), index)
	})
}

func buildProviderInspections(
	catalog map[string]sourceProvider,
	models []model,
	routeSpecs []inspectionRouteSpec,
) (inspectionIndex, []providerInspection, error) {
	routes := append([]inspectionRouteSpec(nil), routeSpecs...)
	sort.Slice(routes, func(i, j int) bool { return routes[i].Provider < routes[j].Provider })

	routeByProvider := make(map[string]inspectionRouteSpec, len(routes))
	providersBySource := make(map[string][]string)
	for _, route := range routes {
		if !validInspectionProviderName(route.Provider) {
			return inspectionIndex{}, nil, fmt.Errorf("invalid inspection provider %q", route.Provider)
		}
		if route.Source == "" {
			return inspectionIndex{}, nil, fmt.Errorf("inspection provider %q has an empty source", route.Provider)
		}
		if _, duplicate := routeByProvider[route.Provider]; duplicate {
			return inspectionIndex{}, nil, fmt.Errorf("duplicate inspection provider %q", route.Provider)
		}
		switch route.Kind {
		case "fixed":
		case "opencode", "copilot":
		default:
			return inspectionIndex{}, nil, fmt.Errorf("inspection provider %q has invalid route kind %q", route.Provider, route.Kind)
		}
		if _, exists := catalog[route.Source]; !exists && route.Kind == "fixed" {
			return inspectionIndex{}, nil, fmt.Errorf("inspection provider %q references missing source %q", route.Provider, route.Source)
		}
		routeByProvider[route.Provider] = route
		providersBySource[route.Source] = append(providersBySource[route.Source], route.Provider)
	}
	for source := range providersBySource {
		sort.Strings(providersBySource[source])
	}

	catalogModels := toCatalogModels(models)
	outputs := make(map[string][]catalogModel, len(routes))
	protocols := make(map[string]map[string]struct{}, len(routes))
	baseURLs := make(map[string]map[string]struct{}, len(routes))
	for _, candidate := range catalogModels {
		if _, exists := routeByProvider[candidate.Provider]; !exists {
			return inspectionIndex{}, nil, fmt.Errorf("generated model %s/%s has no inspection route", candidate.Provider, candidate.ID)
		}
		outputs[candidate.Provider] = append(outputs[candidate.Provider], candidate)
		addInspectionValue(protocols, candidate.Provider, candidate.Protocol)
		addInspectionValue(baseURLs, candidate.Provider, candidate.BaseURL)
	}

	seenSources := make(map[string]struct{})
	var sourceModelCount int
	providers := make([]providerInspection, 0, len(routes))
	summaries := make([]inspectionProviderSummary, 0, len(routes))
	for _, spec := range routes {
		source, sourcePresent := catalog[spec.Source]
		if source.Models == nil {
			source.Models = map[string]sourceModel{}
		}
		after := outputs[spec.Provider]
		if after == nil {
			after = []catalogModel{}
		}
		filtered := len(source.Models) - len(after)
		if filtered < 0 {
			filtered = 0
		}
		sharedWith := make([]string, 0, len(providersBySource[spec.Source])-1)
		for _, provider := range providersBySource[spec.Source] {
			if provider != spec.Provider {
				sharedWith = append(sharedWith, provider)
			}
		}
		route := inspectionRoute{
			Provider:        spec.Provider,
			Source:          spec.Source,
			SourcePresent:   sourcePresent,
			Kind:            spec.Kind,
			SharedWith:      sharedWith,
			Protocol:        spec.Protocol,
			BaseURL:         spec.BaseURL,
			OutputProtocols: sortedInspectionValues(protocols[spec.Provider]),
			OutputBaseURLs:  sortedInspectionValues(baseURLs[spec.Provider]),
		}
		summary := inspectionProviderSummary{
			Provider:       spec.Provider,
			Source:         spec.Source,
			SourcePresent:  sourcePresent,
			InputModels:    len(source.Models),
			OutputModels:   len(after),
			FilteredModels: filtered,
		}
		providers = append(providers, providerInspection{
			Route:  route,
			Before: inspectionInput{Source: spec.Source, Models: source.Models},
			After:  after,
		})
		summaries = append(summaries, summary)
		if _, seen := seenSources[spec.Source]; sourcePresent && !seen {
			seenSources[spec.Source] = struct{}{}
			sourceModelCount += len(source.Models)
		}
	}

	return inspectionIndex{
		SchemaVersion:    inspectionSchemaVersion,
		SourceURL:        modelsDevURL,
		BeforeSemantics:  "subset of models.dev fields parsed by genmodels before filtering and local overrides",
		ProviderCount:    len(providers),
		SourceCount:      len(seenSources),
		SourceModelCount: sourceModelCount,
		OutputModelCount: len(catalogModels),
		Providers:        summaries,
	}, providers, nil
}

func validInspectionProviderName(provider string) bool {
	return provider != "" && provider != "." && provider != ".." && filepath.Base(provider) == provider
}

func addInspectionValue(values map[string]map[string]struct{}, provider, value string) {
	if value == "" {
		return
	}
	providerValues := values[provider]
	if providerValues == nil {
		providerValues = make(map[string]struct{})
		values[provider] = providerValues
	}
	providerValues[value] = struct{}{}
}

func sortedInspectionValues(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func writeInspectionJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func writeInspectionDirectory(target string, build func(string) error) (err error) {
	cleaned := filepath.Clean(target)
	if target == "" || cleaned == "." || filepath.Dir(cleaned) == cleaned {
		return fmt.Errorf("unsafe inspection directory %q", target)
	}
	parent := filepath.Dir(cleaned)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	exists, err := validateExistingInspectionDirectory(cleaned)
	if err != nil {
		return err
	}

	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(cleaned)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	if err := build(temporary); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(temporary, inspectionMarkerName), []byte(inspectionMarkerContent), 0o644); err != nil {
		return err
	}
	if !exists {
		return os.Rename(temporary, cleaned)
	}
	if err := os.RemoveAll(cleaned); err != nil {
		return err
	}
	return os.Rename(temporary, cleaned)
}

func validateExistingInspectionDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("refusing to replace non-directory inspection target %q", path)
	}
	markerPath := filepath.Join(path, inspectionMarkerName)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil || !markerInfo.Mode().IsRegular() {
		return false, fmt.Errorf("refusing to replace unowned inspection directory %q", path)
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil || string(marker) != inspectionMarkerContent {
		return false, fmt.Errorf("refusing to replace unowned inspection directory %q", path)
	}
	return true, nil
}
