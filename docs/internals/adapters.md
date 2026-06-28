# Protocol adapters

Adapters are the edge of the `llm` package. Everything before them is
provider-neutral; everything inside an adapter is allowed to speak one concrete
wire protocol. The package ships two built-ins:

- `openai-completions` for OpenAI-compatible Chat Completions endpoints.
- `anthropic-messages` for Anthropic-compatible Messages endpoints.

## The adapter contract

```go
type ProtocolAdapter interface {
	Protocol() Protocol
	Stream(ctx context.Context, model Model, input Context, options StreamOptions) (<-chan Event, error)
}
```

`Protocol()` is the registry key. `Stream()` owns the full request lifecycle for
one protocol: validate the model, convert messages and tools, build SDK params,
start a goroutine, and emit package events as the provider stream arrives.

## Registry dispatch

`Client.Stream` does not know about OpenAI or Anthropic directly. It asks the
registry for the adapter matching `model.Protocol`, fills in an API key from the
provider environment when needed, and delegates:

```go
adapter, ok := c.registry.Get(model.Protocol)
return adapter.Stream(ctx, model, input, options)
```

`llm.NewClient(registry)` builds a client over a registry. The built-in provider
packages (`llm/openai`, `llm/anthropic`) register their adapters into the package
default registry from an `init` function, so importing a provider — or `llm/all`
for every built-in — makes its protocol available to `llm.Stream` and
`llm.Complete`. A custom client can register another adapter without changing
`StreamOptions` or the shared conversation types.

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
a new adapter. A genuinely different wire protocol needs a new
`ProtocolAdapter`.

Source: [`llm/adapters.go`](https://github.com/ktsoator/or/blob/main/llm/adapters.go),
[`llm/`](https://github.com/ktsoator/or/tree/main/llm/).
