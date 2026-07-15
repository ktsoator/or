# Explicit client

## Purpose

Inject a custom HTTP transport and isolate adapter and provider configuration.

## Core code

```go
transport := http.DefaultTransport.(*http.Transport).Clone()
transport.Proxy = http.ProxyFromEnvironment
transport.MaxIdleConnsPerHost = 20

httpClient := &http.Client{Transport: transport}

adapters := llm.NewAdapterRegistry()
if err := adapters.Register(openai.NewAdapter(httpClient)); err != nil {
	log.Fatal(err)
}

providers := llm.NewBuiltInProviderRegistry()
client := llm.NewClient(adapters, providers)

model := llm.GetModel("deepseek", "deepseek-v4-flash")
response, err := client.Complete(ctx, model,
	llm.Prompt("Reply with OK."), llm.StreamOptions{})
```

Use a normal import:

```go
import "github.com/ktsoator/or/llm/openai"
```

Explicit registration does not rely on side-effect imports. Reuse the `http.Client` and transport instead of creating a connection pool per request.
