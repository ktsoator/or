# LLM recipes

These pages provide minimal paths organized by development task. Complete runnable programs remain under `example/llm/` in the repository.

| Task | Recipe | Primary API |
|---|---|---|
| Send one request | [Basic completion](basic-completion.md) | `LookupModel`, `Complete` |
| Build streaming chat | [Streaming chat](streaming-chat.md) | `Stream`, `Event` |
| Send a screenshot or image | [Image input](vision.md) | `ImageContent`, `Model.Input` |
| Render reasoning | [Reasoning output](reasoning.md) | `Reasoning`, thinking events |
| Run an application tool loop | [Tool loop](tool-loop.md) | `MustTool`, `DecodeToolCall` |
| Switch models between turns | [Model switching](model-switching.md) | `Context.Messages`, `TransformMessages` |
| Connect a proxy or private gateway | [Custom gateway](custom-gateway.md) | `ProviderOverride` |
| Inject an HTTP client | [Explicit client](custom-client.md) | `NewClient`, `NewAdapterRegistry` |

Examples that call a live provider require the corresponding API key. See the [support matrix](../support-matrix.md) for credential variables.
