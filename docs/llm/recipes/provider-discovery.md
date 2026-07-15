# Finding models and checking credentials

An application can list models it can call at startup and check whether each provider has credentials configured. This does not send a model request.

Use `GetRunnableModels` for a model-selection interface and startup checks, rather than `GetModels` alone. The latter also returns models whose protocol adapters are not registered in the current program.

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

## Whether a model is callable

| API | Question answered |
|---|---|
| `GetProviders` | Which provider IDs exist in the built-in model catalog? |
| `GetModels(provider)` | Which models are listed, including models not callable now? |
| `GetRunnableModels(provider)` | Which models can the default protocol-adapter registry route now? |
| `SupportsProtocol(protocol)` | Is an adapter for this protocol registered in the current program? |
| `AuthStatus(provider, env)` | Can the provider resolve a credential, and from where? |

A model being listed in the built-in model catalog does not mean the current program can call it. Calling it also requires a registered adapter for its protocol. `llm/all` registers every built-in protocol; importing one protocol package registers only that protocol.

## Checking credentials

`AuthStatus` reports sources such as `env:DEEPSEEK_API_KEY` or `override` but does not send a request. A configured credential can still be expired, unauthorized for the model, or rejected by the model service.

Do not expose secret values from `GetEnvAPIKey` in diagnostics. Show expected variable names from `APIKeyEnvVars` and missing names from `AuthStatus` instead.
