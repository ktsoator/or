# Providers and models

The catalog is broader than the adapter set. A catalog entry is queryable
metadata, not proof that this package can execute its protocol. Check
[`SupportsProtocol`](api-reference.md#catalog-functions) or use
`GetRunnableModels` before presenting a model as runnable. The current boundary
is maintained in [Protocol support](support-matrix.md), together with provider
IDs, model counts, and credential variables.

The catalog also contains metadata for additional compatible providers and
models. Those entries can be queried and may work through one of the two
protocol adapters, but they have not all been verified against live provider
APIs and are not a support guarantee. Automated tests exercise both adapters
with local mock servers rather than live integration tests for every provider.

Only the key for the provider selected for the current request is resolved. Request-scoped
credentials can also be supplied with `StreamOptions.APIKey` or
`StreamOptions.Env`.

## Discover models

Query the catalog instead of hard-coding model IDs supplied dynamically:

```go
for _, provider := range llm.GetProviders() {
	fmt.Println(provider)
	for _, model := range llm.GetModels(provider) {
		fmt.Printf("  %s: %s\n", model.ID, model.Name)
	}
}

model, ok := llm.LookupModel("xiaomi", "mimo-v2.5")
if !ok {
	log.Fatal("model not found")
}
```

`LookupModel` returns a model and a found flag. `GetModel` is convenient for a
known catalog entry and panics when the provider or model ID does not exist.

The catalog also contains models for protocols that do not have a built-in
adapter yet. Use `SupportsProtocol` to test one protocol, or
`GetRunnableModels` to list only models that the adapters imported by the
current application can serve:

```go
if !llm.SupportsProtocol(model.Protocol) {
	log.Fatalf("protocol %q is not registered", model.Protocol)
}

for _, model := range llm.GetRunnableModels("deepseek") {
	fmt.Println(model.ID)
}
```

Both functions inspect the package default adapter registry. Import the
matching protocol package for side effects, or import `llm/all`, before using
them. `GetModels` remains the unfiltered catalog query.

## Model metadata

A `Model` is also a read-only metadata record. Inspect it to drive UI, enforce
limits, or estimate cost before a request:

| Field | Type | Meaning |
|---|---|---|
| `ID` | `string` | Identifier sent to the provider |
| `Name` | `string` | Human-readable display name |
| `Provider` | `string` | Vendor key, e.g. `anthropic` |
| `Protocol` | `Protocol` | Which adapter handles the model |
| `BaseURL` | `string` | Endpoint base URL |
| `Headers` | `map[string]string` | Default headers merged into each request |
| `Reasoning` | `bool` | Whether the model can produce thinking |
| `Input` | `[]ModelInput` | Accepted modalities: `Text`, `Image` |
| `ContextWindow` | `int64` | Maximum total tokens (input + output) |
| `MaxTokens` | `int64` | Maximum tokens the model may generate |
| `Cost` | `ModelCost` | Per-million-token pricing |
| `Compatibility` | `ModelCompatibility` | Protocol-specific overrides (see below) |

`Reasoning` reports only whether thinking is possible; use
[`SupportedThinkingLevels`](reasoning.md) to read the exact levels a model
accepts rather than the raw `ThinkingLevelMap`.

`Cost` holds prices **per million tokens**, matching how `CalculateCost`
computes a charge:

| Field | Meaning |
|---|---|
| `Input` | Price per million input tokens |
| `Output` | Price per million output tokens |
| `CacheRead` | Price per million cache-read tokens |
| `CacheWrite` | Price per million cache-write tokens |

```go
model, _ := llm.LookupModel("deepseek", "deepseek-v4-flash")
fmt.Printf("%s: %d-token window, $%.2f/M in, $%.2f/M out\n",
	model.Name, model.ContextWindow, model.Cost.Input, model.Cost.Output)
```

Catalog prices are static metadata bundled with this repository. They may lag
provider price changes and do not include account-specific discounts, taxes, or
provider-side rounding. Use the provider invoice as the billing authority.

See [Reading responses](results.md) for the matching `Usage` and `UsageCost`
records on a completed request.

## Custom and compatible endpoints

Any endpoint implementing one of the built-in protocols can be used by
constructing a `Model` directly and setting `BaseURL`. This covers local servers
such as Ollama, vLLM, and LM Studio, as well as private model gateways:

```go
model := llm.Model{
	ID:            "qwen2.5-coder:7b",
	Name:          "Qwen2.5 Coder 7B",
	Provider:      "ollama",
	Protocol:      llm.ProtocolOpenAICompletions,
	BaseURL:       "http://localhost:11434/v1",
	Input:         []llm.ModelInput{llm.Text},
	ContextWindow: 32768,
	MaxTokens:     4096,
}

events, err := llm.Stream(ctx, model, input, llm.StreamOptions{APIKey: "ollama"})
```

Endpoint-specific behavior—reasoning field names, cache-control support, and
similar differences—is configured through `Model.Compatibility` with
`OpenAICompletionsCompatibility` or `AnthropicMessagesCompatibility`. Set only
the fields that differ from the default; each is a pointer so an unset field
leaves the adapter's behavior unchanged.

```go
supports := func(b bool) *bool { return &b }

// OpenAI-compatible endpoint that names its cap "max_completion_tokens"
// and accepts a reasoning effort field.
model.Compatibility = &llm.OpenAICompletionsCompatibility{
	MaxTokensField:          "max_completion_tokens",
	SupportsReasoningEffort: supports(true),
}

// Anthropic-compatible endpoint that does not support cache control.
model.Compatibility = &llm.AnthropicMessagesCompatibility{
	SupportsCacheControl: supports(false),
}
```

The compatibility records expose these fields:

| Type | Field | Effect |
|---|---|---|
| `OpenAICompletionsCompatibility` | `MaxTokensField` | Changes the JSON field used for the output-token cap |
| `OpenAICompletionsCompatibility` | `SupportsReasoningEffort` | Enables or disables the OpenAI reasoning-effort field |
| `OpenAICompletionsCompatibility` | `SupportsDeveloperRole` | Controls whether developer-role messages may be sent |
| `OpenAICompletionsCompatibility` | `SupportsStore` | Controls whether the adapter sends `store=false` |
| `OpenAICompletionsCompatibility` | `SupportsStrictMode` | Controls strict mode on tool definitions |
| `OpenAICompletionsCompatibility` | `RequiresReasoningContentOnAssistantMessages` | Requires reasoning content when assistant turns are replayed |
| `OpenAICompletionsCompatibility` | `RequiresThinkingAsText` | Replays thinking as ordinary text instead of a reasoning field |
| `OpenAICompletionsCompatibility` | `ThinkingFormat` | Selects the endpoint-specific reasoning representation |
| `OpenAICompletionsCompatibility` | `ZAIToolStream` | Adds Z.AI's `tool_stream` field |
| `AnthropicMessagesCompatibility` | `SupportsTemperature` | Enables or disables temperature |
| `AnthropicMessagesCompatibility` | `SupportsCacheControl` | Enables or disables message cache control |
| `AnthropicMessagesCompatibility` | `SupportsCacheControlTools` | Enables or disables cache control on tools |
| `AnthropicMessagesCompatibility` | `ForceAdaptiveThinking` | Forces adaptive thinking |
| `AnthropicMessagesCompatibility` | `AllowEmptySignature` | Permits an empty replayed thinking signature |

All Boolean compatibility fields are pointers. `nil` means adapter default;
`false` is an explicit override. The accepted `ThinkingFormat` strings are
defined by the current adapters; the package does not publish an open enum for
arbitrary providers.

For a wire protocol that is neither OpenAI-compatible nor
Anthropic-compatible, implement a [custom protocol adapter](extending.md).

## Provider configuration and status

The package keeps a provider registry next to the model catalog. The catalog
lists a provider's models; the registry holds its configuration: the environment
variables that supply its key, and any override applied to its requests. The
package-level `Stream` and `Complete` route through the default registry, so
status queries and overrides take effect without building your own client.

### Check whether a provider is configured

`AuthStatus` reports whether a key resolves and which source it came from,
without sending a request.

```go
registry := llm.DefaultProviderRegistry()

status, ok := registry.AuthStatus("deepseek", nil)
if ok && !status.Configured {
	fmt.Printf("%s not configured; set one of %v\n", status.Label, status.Missing)
}
// A configured provider reports its source, e.g. "env:DEEPSEEK_API_KEY".
```

### Redirect a provider's requests

`SetOverride` sets a base URL, API key, or headers for every request to a
provider, so a proxy or gateway needs no change to each `Model`.

```go
proxy := "https://proxy.example.com/deepseek/v1"
registry.SetOverride("deepseek", llm.ProviderOverride{
	BaseURL: &proxy,
	Headers: map[string]string{"X-Team": "infra"},
})
// Every deepseek model now streams through the proxy.
```

`SetOverride` stores an independent snapshot, so the values, maps, and
request-scoped environment passed to it may be reused or mutated after the call.
Set overrides at startup when possible. Credential precedence is listed in
[request configuration](configuration.md).

### Register a custom provider

`Register` adds a provider the catalog does not ship. It resolves its key from
its own environment variables and can be overridden like a built-in one, which
is the alternative to passing a bare `Model` for a local server.

```go
registry.Register(llm.NewSpecProvider(llm.ProviderSpec{
	ID:      "local",
	Name:    "Local LLM",
	EnvKeys: []string{"LOCAL_API_KEY"},
	Models: []llm.Model{{
		ID:       "qwen2.5-coder:7b",
		Provider: "local",
		Protocol: llm.ProtocolOpenAICompletions,
		BaseURL:  "http://localhost:11434/v1",
		Input:    []llm.ModelInput{llm.Text},
	}},
}))
```

`NewSpecProvider` builds a provider from an independent snapshot of the supplied
data, including model configuration. Vendors that need per-request logic such
as OAuth refresh are not covered by the spec type yet.

`ProviderRegistry` also exposes `Get`, `Providers`, `Resolve`, `Models`,
`Override`, and `ClearOverride`. Applications that need isolated registries or
explicit clients should use [Clients and registries](clients-and-registries.md)
instead of mutating the package defaults.
