# Clients and registries

Package-level `llm.Stream` and `llm.Complete` use one global default `Client`. That client combines an adapter registry with a built-in provider registry. Most applications only need a side-effect import for the protocol adapter and the package-level entry points.

Use an explicit `Client` when you need to:

- isolate provider overrides by subsystem or tenant;
- inject an `http.Client`, transport, proxy, or TLS configuration;
- avoid mutating package-global state in tests;
- register only approved protocols;
- install a custom `ProtocolAdapter`.

## The three registries

| Type | Stores | Primary key | Request role |
|---|---|---|---|
| `AdapterRegistry` | `ProtocolAdapter` values | `Protocol` | Selects a wire-protocol implementation from `Model.Protocol` |
| `ProviderRegistry` | Provider configuration and overrides | `Model.Provider` | Resolves key, URL, and headers |
| `ModelRegistry` | `Model` metadata | provider + model ID | Supports discovery; `Client` does not depend on it directly |

The registries do not replace one another. Registering a model does not register a protocol adapter or provider credentials.

## Default client

`llm/default.go` creates the following package state:

```text
defaultRegistry         = NewAdapterRegistry()
defaultProviderRegistry = NewBuiltInProviderRegistry()
defaultClient           = NewClient(defaultRegistry, defaultProviderRegistry)
```

`llm/openai` and `llm/anthropic` register themselves with the default adapter registry from `init`. Only imported protocols appear in `SupportsProtocol`.

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

response, err := llm.Complete(ctx, model, input, options)
```

`DefaultProviderRegistry` returns the provider registry used by the default client. `SetOverride` on that registry affects later package-level requests for that provider throughout the process.

## Explicit client

This program registers both built-in adapters and uses an independent provider registry:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/anthropic"
	"github.com/ktsoator/or/llm/openai"
)

func main() {
	adapters := llm.NewAdapterRegistry()
	if err := adapters.Register(openai.NewAdapter(nil)); err != nil {
		log.Fatal(err)
	}
	if err := adapters.Register(anthropic.NewAdapter(nil)); err != nil {
		log.Fatal(err)
	}

	providers := llm.NewBuiltInProviderRegistry()
	client := llm.NewClient(adapters, providers)

	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok {
		log.Fatal("model not found")
	}

	response, err := client.Complete(
		context.Background(), model,
		llm.Prompt("Reply with OK."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

`NewClient(adapters, nil)` skips the provider registry but still applies the legacy environment-variable mapping for `Model.Provider`. Pass `StreamOptions.APIKey` when a custom provider has no legacy mapping.

## Custom HTTP client

Both built-in adapters accept `*http.Client`:

```go
transport := http.DefaultTransport.(*http.Transport).Clone()
transport.MaxIdleConns = 100
transport.MaxIdleConnsPerHost = 20

httpClient := &http.Client{Transport: transport}

adapters := llm.NewAdapterRegistry()
if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
	log.Fatal(err)
}

client := llm.NewClient(adapters, llm.NewBuiltInProviderRegistry())
```

`openai.NewAdapter(nil)` uses `http.DefaultClient`. `anthropic.NewAdapter(nil)` lets the Anthropic SDK use its default client. Request-level `StreamOptions.Timeout` still applies at the SDK request layer.

Share an `http.Client` to reuse connection pools. Do not create a new transport per model request.

## ProviderRegistry

### Construction

```go
empty := llm.NewProviderRegistry()
builtIn := llm.NewBuiltInProviderRegistry()
defaultRegistry := llm.DefaultProviderRegistry()
```

- `NewProviderRegistry` returns an empty registry.
- `NewBuiltInProviderRegistry` derives provider configuration from the embedded catalog.
- `DefaultProviderRegistry` returns the instance used by the package-level client.

### Query and mutation

| Method | Purpose |
|---|---|
| `Register(provider)` | Add or replace a provider |
| `Get(providerID)` | Look up a provider |
| `Providers()` | Return providers sorted by ID |
| `SetOverride(providerID, override)` | Override URL, key, headers, or environment |
| `ClearOverride(providerID)` | Remove an override |
| `AuthStatus(providerID, env)` | Inspect credential resolution |
| `ResolveRequest(model, options)` | Compute the model and options seen by the adapter |

`ProviderRegistry` protects its maps with a read/write lock. Registration and overrides clone their inputs. Requests that have already resolved their configuration are unaffected by later changes.

## ModelRegistry

`ModelRegistry` is an optional application-owned model catalog:

```go
models := llm.NewModelRegistry()
if err := models.Register(llm.Model{
	ID:       "local-model",
	Name:     "Local Model",
	Provider: "local",
	Protocol: llm.ProtocolOpenAICompletions,
	BaseURL:  "http://localhost:8080/v1",
	Input:    []llm.ModelInput{llm.Text},
}); err != nil {
	log.Fatal(err)
}

model, ok := models.Get("local", "local-model")
```

| Method | Purpose |
|---|---|
| `Register(model)` | Validate and add or replace a model |
| `Get(provider, modelID)` | Return a defensive copy |
| `Providers()` | Return provider IDs in lexical order |
| `Models(provider)` | Return models ordered by model ID |

Package-level `LookupModel` and `GetModel` always query the built-in catalog, not an application-created `ModelRegistry`. A model obtained from a custom registry can still be passed directly to either client form.

## Concurrency and lifecycle

- `AdapterRegistry`, `ProviderRegistry`, and `ModelRegistry` support concurrent reads and mutations.
- Reuse the adapter's `http.Client` for the client lifetime.
- `Client` has no `Close` method. Each adapter closes its request stream when the consumer goroutine exits.
- With `Stream`, continue receiving until the event channel closes. If business logic no longer needs deltas, still drain the channel.
- Provider overrides are in-memory configuration and are not persisted.
- `DefaultProviderRegistry` is shared global state. Tests that modify it should call `ClearOverride`, or use an explicit client.

## Selection guide

| Scenario | Recommended form |
|---|---|
| One application with fixed providers | Package-level `Stream`/`Complete` |
| Per-request tenant key | Either client form with `StreamOptions.APIKey` |
| Per-tenant URL or headers | One `ProviderRegistry` and `Client` per tenant |
| Custom proxy, TLS, or transport | Explicit adapter with a custom `http.Client` |
| Unit tests or mock servers | Explicit client to avoid default-registry mutation |
| New wire protocol | Custom `ProtocolAdapter` |
