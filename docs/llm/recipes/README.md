# LLM task guides

These guides are complete development paths rather than isolated API snippets. Each page explains when to use the feature, how the request flows through `llm`, a runnable program, important parameters, expected results, failure behavior, and production constraints.

## Start here

| Goal | Guide | Primary API |
|---|---|---|
| Send one configured request | [Basic completion](basic-completion.md) | `LookupModel`, `Complete` |
| Render output as it arrives | [Streaming chat](streaming-chat.md) | `Stream`, `Event` |
| Maintain and persist history | [Conversation persistence](conversation-persistence.md) | `Context.Messages`, JSON helpers |
| Send a screenshot or image | [Image input](vision.md) | `ImageContent`, `Model.Input` |
| Request and display reasoning | [Reasoning output](reasoning.md) | `Reasoning`, thinking events |
| Execute structured tools | [Tool loop](tool-loop.md) | `MustTool`, `DecodeToolCall` |
| Change models between turns | [Model switching](model-switching.md) | `TransformMessages` |

## Configuration and operations

| Goal | Guide | Primary API |
|---|---|---|
| Discover runnable models and credentials | [Model and auth discovery](provider-discovery.md) | `GetRunnableModels`, `AuthStatus` |
| Route through a proxy or private endpoint | [Custom gateway](custom-gateway.md) | `ProviderOverride`, `Model.BaseURL` |
| Isolate configuration or inject transport | [Explicit client](custom-client.md) | `NewClient`, registries |
| Add request tracing or body rewrites | [Observability hooks](observability.md) | `OnRequest`, `RewriteRequest`, `OnResponse` |
| Handle failures consistently | [Error handling](error-handling.md) | `StopReason`, `IsContextOverflow` |
| Test without a provider account | [Mock-provider testing](mock-testing.md) | `httptest`, explicit `Model` |

## Common setup

Examples use Go 1.24 or later and the current module version:

```sh
go get github.com/ktsoator/or/llm@latest
```

Every request needs both a model and a registered adapter for its protocol. Import `llm/openai` for `openai-completions`, `llm/anthropic` for `anthropic-messages`, or `llm/all` for both. Catalog membership alone does not make a model runnable.

Live-provider examples use environment variables shown in the [protocol support matrix](../support-matrix.md). Generated text, usage, and provider response IDs vary by account and model version.

## Reading the examples

- `Complete` is used when only the terminal `AssistantMessage` matters.
- `Stream` is used when text, reasoning, or tool deltas must be rendered live.
- The application owns message persistence and tool execution.
- Always consume a stream until its channel closes, including after cancellation.
- Catalog cost is an estimate; provider billing remains authoritative.

Repository programs under `example/llm/` remain useful as compact smoke tests. These task guides contain the surrounding design and operational guidance required for integration work.
