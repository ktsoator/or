package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

//go:generate go run ./cmd/genmodels -output catalog.generated.json

// generatedCatalogJSON is checked into the repository and embedded so normal
// builds and application startup never depend on the network or working directory.
//
//go:embed catalog.generated.json
var generatedCatalogJSON []byte

var builtInModelRegistry = newBuiltInModelRegistry()

func newBuiltInModelRegistry() *ModelRegistry {
	registry := NewModelRegistry()
	for _, model := range builtInModels() {
		if err := registry.Register(model); err != nil {
			panic(err)
		}
	}
	return registry
}

func builtInModels() []Model {
	var providers map[string][]Model
	if err := json.Unmarshal(generatedCatalogJSON, &providers); err != nil {
		panic(fmt.Errorf("decode embedded model catalog: %w", err))
	}

	providerIDs := make([]string, 0, len(providers))
	total := 0
	for provider, models := range providers {
		providerIDs = append(providerIDs, provider)
		total += len(models)
	}
	sort.Strings(providerIDs)

	all := make([]Model, 0, total)
	for _, provider := range providerIDs {
		models := providers[provider]
		for index := range models {
			if models[index].Provider != "" && models[index].Provider != provider {
				panic(fmt.Errorf(
					"model catalog provider %q does not match model %q provider %q",
					provider,
					models[index].ID,
					models[index].Provider,
				))
			}
			models[index].Provider = provider
		}
		all = append(all, models...)
	}
	return all
}
