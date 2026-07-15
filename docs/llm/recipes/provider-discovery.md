# Model and auth discovery

## What this builds

A diagnostic command lists only models routable by the adapters imported into the process and reports provider credential status without sending a request.

This is the correct basis for model-selection UIs and startup checks. `GetModels` alone includes catalog entries for protocols that have no built-in adapter.

## Complete program

```go
package main

import (
	"fmt"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/all"
)

func main() {
	providers := llm.GetProviders()
	registry := llm.DefaultProviderRegistry()
	for _, providerID := range providers {
		models := llm.GetRunnableModels(providerID)
		if len(models) == 0 {
			continue
		}
		status, ok := registry.AuthStatus(providerID, nil)
		if !ok {
			fmt.Printf("%s: provider is not registered\n", providerID)
			continue
		}
		fmt.Printf("%s configured=%t source=%q missing=%v\n",
			providerID, status.Configured, status.Source, status.Missing)
		for _, model := range models {
			fmt.Printf("  %s protocol=%s context=%d image=%v reasoning=%t\n",
				model.ID, model.Protocol, model.ContextWindow,
				model.Input, model.Reasoning)
		}
	}
}
```

## Catalog and runtime are separate

| API | Question answered |
|---|---|
| `GetProviders` | Which provider IDs exist in the embedded catalog? |
| `GetModels(provider)` | Which catalog models exist, including unimplemented protocols? |
| `GetRunnableModels(provider)` | Which models can the default adapter registry route now? |
| `SupportsProtocol(protocol)` | Was an adapter for this protocol imported? |
| `AuthStatus(provider, env)` | Can this provider resolve a credential, and from where? |

`AuthStatus` reports sources such as `env:DEEPSEEK_API_KEY` or `override` but does not send a request. A configured credential can still be expired, unauthorized for the model, or rejected by the endpoint.

Do not expose secret values from `GetEnvAPIKey` in diagnostics. Show expected variable names with `APIKeyEnvVars` and missing names from `AuthStatus` instead.
