# Protocol adapters

Adapters are the edge of the `llm` package. Everything before them is
provider-neutral; everything inside an adapter is allowed to speak one concrete
wire protocol. The package ships two built-ins:

- `openai-completions` for OpenAI-compatible Chat Completions endpoints.
- `anthropic-messages` for Anthropic-compatible Messages endpoints.

## The adapter contract

```go
type ProtocolAdapter interface {
	// Protocol returns the registry key used to select this adapter.
	Protocol() Protocol

	// Stream emits response events for the given model and conversation context.
	Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error)
}
```

`Protocol()` is the registry key. `Stream()` owns the full request lifecycle for
one protocol: validate the model, transform and convert the history, build SDK
params, start a goroutine, and emit package events as the provider stream
arrives.

## Registry dispatch

`Client.Stream` does not know about OpenAI or Anthropic directly. It asks the
registry for the adapter matching `model.Protocol` and delegates. The registry is
a concurrency-safe map; `Register` adds or replaces the adapter for its protocol:

```go
func (registry *AdapterRegistry) Register(adapter ProtocolAdapter) error {
	if adapter == nil {
		return errors.New("protocol adapter is nil")
	}

	protocol := adapter.Protocol()
	if protocol == "" {
		return errors.New("protocol adapter protocol is empty")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.adapters[protocol] = adapter
	return nil
}
```

## Registration on import

The built-in provider packages register their adapters into the package default
registry from an `init` function, so importing a provider — or `llm/all` for
every built-in — makes its protocol available to `llm.Stream` and `llm.Complete`:

```go
func init() {
	if err := llm.Register(NewAdapter(nil)); err != nil {
		panic(err)
	}
}
```

A caller that prefers explicit wiring skips the default registry, builds one with
`NewAdapterRegistry`, registers adapters into it, and passes it to `NewClient`.

## Building the SDK client

`Stream` maps the neutral `StreamOptions` onto the vendor SDK's request options.
`buildClient` in the Anthropic adapter shows the shape: base URL, retries, and
timeout become SDK options, and the observation hooks become middleware:

```go
func buildClient(httpClient *http.Client, model llm.Model, options llm.StreamOptions) sdk.Client {
	clientOptions := []option.RequestOption{
		option.WithAPIKey(options.APIKey),
	}
	if httpClient != nil {
		clientOptions = append(clientOptions, option.WithHTTPClient(httpClient))
	}
	if model.BaseURL != "" { // (1)!
		clientOptions = append(clientOptions, option.WithBaseURL(model.BaseURL))
	}
	if options.MaxRetries != nil {
		clientOptions = append(clientOptions, option.WithMaxRetries(*options.MaxRetries))
	}
	if options.Timeout > 0 {
		clientOptions = append(clientOptions, option.WithRequestTimeout(options.Timeout))
	}
	if options.OnRequest != nil { // (2)!
		clientOptions = append(clientOptions, option.WithMiddleware(onRequestMiddleware(options.OnRequest)))
	}
	if options.RewriteRequest != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(rewriteRequestMiddleware(options.RewriteRequest)))
	}
	if options.OnResponse != nil {
		clientOptions = append(clientOptions, option.WithMiddleware(onResponseMiddleware(options.OnResponse)))
	}
	for name, value := range mergedHeaders(model, options) { // (3)!
		clientOptions = append(clientOptions, option.WithHeader(name, value))
	}
	return sdk.NewClient(clientOptions...)
}
```

1.  `BaseURL` is what lets a compatible vendor reuse this adapter against its own
    endpoint.
2.  `OnRequest`, `RewriteRequest`, and `OnResponse` are installed as SDK
    middleware, so each fires once per attempt — retries included — and
    `RewriteRequest` can patch the serialized body before it is sent.
3.  Request headers override model default headers of the same name.

## What an adapter translates

Both built-ins follow the same shape:

1. Validate that `model.Protocol` and `model.Compatibility` match the adapter.
2. Call `TransformMessages` before serializing history for the target model.
3. Convert neutral content blocks into the provider's request message format.
4. Convert `ToolDefinition` into the provider's native tool schema wrapper.
5. Map neutral options, such as reasoning and max tokens, into provider fields.
6. Consume the provider stream and rebuild an `AssistantMessage` plus events.

The protocol-specific knobs stay nested in `StreamOptions.ProtocolOptions`.
OpenAI-compatible models accept `OpenAICompletionsStreamOptions`; Anthropic
models accept `AnthropicStreamOptions`. The shared options validator rejects a
protocol mismatch before any HTTP request is sent.

## Compatible vendors

`Model.BaseURL`, `Model.Headers`, and `Model.Compatibility` let a vendor reuse an
adapter even when it is not the reference provider. For example, an
OpenAI-compatible provider can point `BaseURL` at its own endpoint and set
compatibility flags for details such as `max_tokens` versus
`max_completion_tokens`, strict tool support, or the field name used for
reasoning content.

The result is that adding a compatible provider is usually a catalog change, not
a new adapter. A genuinely different wire protocol needs a new `ProtocolAdapter`.

Source: [`llm/adapters.go`](https://github.com/ktsoator/or/blob/main/llm/adapters.go),
[`llm/anthropic/adapter.go`](https://github.com/ktsoator/or/blob/main/llm/anthropic/adapter.go),
[`llm/openai/`](https://github.com/ktsoator/or/tree/main/llm/openai).
