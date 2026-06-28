package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:generate go run ../internal/genmodels -output catalog.generated.json

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
	var models []Model
	if err := json.Unmarshal(generatedCatalogJSON, &models); err != nil {
		panic(fmt.Errorf("decode embedded model catalog: %w", err))
	}
	return models
}
