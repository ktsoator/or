# Explicit client

## What this builds

An application creates its own HTTP transport, adapter registry, provider registry, and `Client`. This avoids package-global provider overrides and makes network dependencies explicit.

Use this form for custom TLS or proxies, tenant isolation, tests, restricted protocol allowlists, or a custom adapter.

## Complete program

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ktsoator/or/llm"
	"github.com/ktsoator/or/llm/openai"
)

func main() {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	httpClient := &http.Client{Transport: transport}

	adapters := llm.NewAdapterRegistry()
	if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
		log.Fatal(err)
	}
	providers := llm.NewBuiltInProviderRegistry()
	client := llm.NewClient(adapters, providers)

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := client.Complete(context.Background(), model,
		llm.Prompt("Reply with OK."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

This uses a normal import rather than a side-effect import because the program constructs and registers the adapter itself.

## Registry responsibilities

| Component | Responsibility |
|---|---|
| `AdapterRegistry` | Map `Model.Protocol` to a wire-protocol implementation |
| `ProviderRegistry` | Resolve credentials, URL overrides, and headers |
| `ModelRegistry` | Optional application-owned model discovery; not required by `Client` |
| `http.Client` | Connection pooling, proxy, TLS, and transport-level behavior |

`NewClient(adapters, nil)` skips the provider registry but retains legacy environment lookup for known `Model.Provider` values. A nil adapter registry causes requests to fail.

## Lifecycle and concurrency

- Registries support concurrent reads and mutations and return defensive copies where documented.
- Reuse `http.Client` and its transport. Creating one transport per request discards connection pooling.
- `llm.Client` has no `Close` method. The application owns the supplied HTTP transport; call `CloseIdleConnections` during application shutdown if required.
- One client per tenant is reasonable when provider URLs or headers differ. Per-request API keys alone do not require separate clients.
- Register only required protocols when binary dependencies or endpoint policy must be restricted.

For custom provider configuration, register `NewSpecProvider` on the isolated provider registry. For a new wire protocol, see [Custom protocols](../extending.md).
