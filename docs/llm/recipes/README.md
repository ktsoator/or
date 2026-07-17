# LLM guides

Choose a page by the task you need to complete. These guides own complete
programs, integration steps, and application policy; the pages under API and
concepts own type, field, and behavioral contracts.

## Requests and conversations

| Capability | Guide | Primary API |
|---|---|---|
| Send one configured request | [Basic completion](basic-completion.md) | `LookupModel`, `Complete` |
| Render output as it arrives | [Streaming responses](streaming-chat.md) | `Stream`, `Event` |
| Save and restore history | [Saving and restoring conversations](conversation-persistence.md) | `Context.Messages`, JSON helpers |
| Send a screenshot or image | [Sending images](vision.md) | `ImageContent`, `Model.Input` |
| Request and display reasoning | [Requesting reasoning](reasoning.md) | `Reasoning`, thinking events |
| Execute structured tools | [Executing tool calls](tool-loop.md) | `MustTool`, `DecodeToolCall` |
| Change models between turns | [Changing models in a conversation](model-switching.md) | `TransformMessages` |

## Integration, configuration, and testing

| Capability | Guide | Primary API |
|---|---|---|
| Discover runnable models and credentials | [Finding models and checking credentials](provider-discovery.md) | `GetRunnableModels`, `AuthStatus` |
| Route through a proxy or private endpoint | [Connecting a custom model service](custom-gateway.md) | `ProviderOverride`, `Model.BaseURL` |
| Isolate configuration or inject transport | [Creating a custom client](custom-client.md) | `NewClient`, registries |
| Add request tracing or body rewrites | [Recording and rewriting requests](observability.md) | `OnRequest`, `RewriteRequest`, `OnResponse` |
| Handle failures consistently | [Handling request failures](error-handling.md) | `StopReason`, `IsContextOverflow` |
| Test without a provider account | [Testing with a local mock server](mock-testing.md) | `httptest`, explicit `Model` |

## Before running an example

Examples require Go 1.25 or later:

```sh
go get github.com/ktsoator/or/llm@latest
```

Before a call, provide a model and register an adapter for its protocol. Import `llm/openai` for `openai-completions`, `llm/anthropic` for `anthropic-messages`, or `llm/all` for both. A model being listed in the built-in model catalog does not mean the corresponding adapter is registered in the current program.

Live-provider examples use environment variables shown in [Protocol and provider status](../support-matrix.md). Generated text, usage, and provider response IDs vary by account and model version.

## Conventions used by the examples

- Call `Complete` when only the final `AssistantMessage` is needed.
- Call `Stream` when text, reasoning, or tool-call deltas must be handled as they arrive.
- The application owns conversation persistence and actual tool execution.
- Continue consuming stream events until the channel closes, including after context cancellation.
- `Usage.Cost` is estimated from built-in model catalog prices and is not a replacement for provider billing.

Programs under `example/llm/` are useful for quick verification. These pages add the parameters, boundaries, and runtime constraints needed for integration.
