# Custom gateway

## Provider-wide redirect

Route every request for a provider through a proxy without editing each catalog `Model`:

```go
registry := llm.DefaultProviderRegistry()

baseURL := "https://gateway.example.com/deepseek/v1"
apiKey := "gateway-key"
registry.SetOverride("deepseek", llm.ProviderOverride{
	BaseURL: &baseURL,
	APIKey:  &apiKey,
	Headers: map[string]string{
		"X-Tenant": "tenant-a",
	},
})
defer registry.ClearOverride("deepseek")
```

`SetOverride` affects later requests using that registry. The default registry is process-global. Use separate registries and clients when tenants require different URLs.

## One compatible model

```go
model := llm.Model{
	ID:            "local-model",
	Name:          "Local Model",
	Provider:      "local",
	Protocol:      llm.ProtocolOpenAICompletions,
	BaseURL:       "http://localhost:8080/v1",
	Input:         []llm.ModelInput{llm.Text},
	ContextWindow: 32768,
	MaxTokens:     4096,
}

response, err := llm.Complete(ctx, model, llm.Prompt("hello"),
	llm.StreamOptions{APIKey: "local-key"})
```

The endpoint must implement the selected wire protocol. Returning superficially similar JSON is not sufficient compatibility.
