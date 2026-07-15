# Creating a custom client

Package-level `Complete` and `Stream` use a default client and default registries. Create an independent `llm.Client` when the application must control network connections, provider configuration, or available protocols.

Use this approach for custom TLS or proxies, tenant isolation, tests that must avoid global state, and protocol allowlists.

## Before running the example

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

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

This uses a normal import because the program calls `openai.NewAdapter` and registers the protocol adapter itself. The package-level default client does not use this registry.

## Components of a dedicated client

The example creates an `AdapterRegistry`, `ProviderRegistry`, and `http.Client`
before passing them to `NewClient`. See
[Clients and registries](../clients-and-registries.md#the-three-registries) for
the canonical ownership, request participation, and concurrency rules.

`ModelRegistry` performs model lookup and is not a `NewClient` argument.
`NewClient(adapters, nil)` skips the provider registry but retains legacy
environment lookup for known `Model.Provider` values. A nil adapter registry
causes requests to fail.

## Isolation and lifecycle

- Registries support concurrent reads and mutations.
- Reuse `http.Client` and `http.Transport`. Creating them per request prevents connection-pool reuse.
- `llm.Client` has no `Close` method. The application owns the supplied HTTP transport; call `CloseIdleConnections` during application shutdown if required.
- Separate clients are reasonable when tenants use different service addresses, headers, or network policies. API keys alone can be passed in request options and do not require separate clients.
- Register only required protocols when binary dependencies or endpoint policy must be restricted.

For custom provider configuration, register `NewSpecProvider` on the isolated provider registry. For a new wire protocol, see [Custom protocols](../extending.md).
