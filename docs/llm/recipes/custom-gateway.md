# Custom gateway

## Choosing the integration form

Use a provider override when all requests for an existing provider should use one gateway. Construct a `Model` directly when only one compatible endpoint is needed or the provider is not cataloged. Neither form adds a new wire protocol.

## Complete provider-override program

The gateway URL and key come from application configuration rather than source code:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	baseURL := os.Getenv("GATEWAY_BASE_URL")
	apiKey := os.Getenv("GATEWAY_API_KEY")
	if baseURL == "" || apiKey == "" {
		log.Fatal("set GATEWAY_BASE_URL and GATEWAY_API_KEY")
	}

	registry := llm.DefaultProviderRegistry()
	registry.SetOverride("deepseek", llm.ProviderOverride{
		BaseURL: &baseURL,
		APIKey:  &apiKey,
		Headers: map[string]string{"X-Tenant": "team-a"},
	})
	defer registry.ClearOverride("deepseek")

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := llm.Complete(context.Background(), model,
		llm.Prompt("Reply with OK."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

## Precedence

| Setting | Highest to lowest |
|---|---|
| API key | request `APIKey` → override `APIKey` → request `Env` → override `Env` → process environment |
| Base URL | provider override → `Model.BaseURL` |
| Header per name | request → override → provider spec → model |

`SetOverride` stores a snapshot. Later mutation of the input maps does not change the registered override. Requests that already resolved configuration are not affected by later updates.

## One compatible endpoint

```go
model := llm.Model{
	ID: "local-model", Name: "Local Model", Provider: "local",
	Protocol: llm.ProtocolOpenAICompletions,
	BaseURL: "http://localhost:8080/v1",
	Input: []llm.ModelInput{llm.Text}, MaxTokens: 4096,
}
response, err := llm.Complete(ctx, model, llm.Prompt("hello"),
	llm.StreamOptions{APIKey: "local-key"})
```

The endpoint must implement the selected protocol's streaming and error behavior, not merely return similar JSON. Configure `Model.Compatibility` for known dialect differences. Implement `ProtocolAdapter` only for a genuinely different wire protocol.

## Operational constraints

- `DefaultProviderRegistry` is process-global. Avoid tenant-specific overrides on a shared default registry.
- Do not accept arbitrary user-supplied base URLs without SSRF controls and network allowlists.
- Preserve TLS verification; custom certificates belong on an explicit `http.Transport`.
- Test tools, reasoning, usage, retries, and error streams against the actual gateway.
- Clear test overrides or use an isolated client to prevent cross-test leakage.
