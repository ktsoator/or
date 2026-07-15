# Connecting a custom model service

Use this page to send model calls to a proxy, private deployment, or compatible service. A model service address is its public HTTP API address, for example `https://gateway.example.com/v1`.

## Choosing an integration method

| Situation | Method |
|---|---|
| Every call to an existing provider uses one service | Use `ProviderOverride` |
| Calling one compatible service, or a service absent from the built-in model catalog | Construct a `Model` directly |
| The service uses request and response formats outside framework protocols | Implement `ProtocolAdapter` |

The first two methods reuse an existing protocol adapter and do not require a new one.

## Overriding an existing provider

Read the service address and API key from application configuration, not source code:

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

## Override scope and precedence

`ProviderOverride` can replace credentials, service address, headers, and
environment values. The complete precedence against request options, model
fields, and process environment is maintained only in
[Request options](../configuration.md#supply-credentials-per-request) and
[Models and providers](../providers.md#redirect-a-providers-requests).

`SetOverride` stores a snapshot. Later mutation of the input maps does not change the registered override. Requests already resolving configuration are not affected by later updates.

## Connecting one compatible service

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

The model service must implement the selected protocol's request, streaming response, and error behavior; returning similar JSON is not enough to guarantee compatibility. Configure known field differences with `Model.Compatibility`. Implement `ProtocolAdapter` only when request and response formats fall outside existing protocols.

## Compatibility and security boundaries

- `DefaultProviderRegistry` is process-global. Avoid tenant-specific overrides on a shared default registry.
- Do not accept user-supplied service addresses without SSRF controls and network allowlists.
- Preserve TLS verification; custom certificates belong on an explicit `http.Transport`.
- Test tools, reasoning, usage, retries, and error streams against the actual gateway.
- Clear test overrides or use an isolated client to prevent cross-test leakage.
