# Capabilities

`github.com/ktsoator/or/llm` is a stateless LLM protocol translation layer. It normalizes requests, messages, streamed events, tool calls, reasoning content, and usage. It does not store conversations or execute tools.

This page maps development tasks to the public API. See the [API reference](api-reference.md) for the complete type and method index.

## Tasks and APIs

| Development task | Primary API | Behavior | Guide |
|---|---|---|---|
| One-shot text generation | `Complete`, `Prompt` | Collect a stream and return an `AssistantMessage` | [Basic completion](recipes/basic-completion.md) |
| Add a system prompt | `PromptWithSystem` | Set `Context.SystemPrompt` for a request | [Conversations](conversations.md) |
| Build a streaming UI | `Stream`, `EventTextDelta` | Render text, reasoning, and tool arguments incrementally | [Streaming chat](recipes/streaming-chat.md) |
| Continue a conversation | `Context.Messages`, `UserText` | Store history in the caller and resend it each turn | [Conversation persistence](recipes/conversation-persistence.md) |
| Send images | `UserImage`, `ImageContent` | Send base64 images; text-only targets receive placeholders | [Image input](recipes/vision.md) |
| Switch model or protocol | `LookupModel`, `TransformMessages` | Reuse history and adapt it for the target model | [Model switching](recipes/model-switching.md) |
| Display reasoning | `StreamOptions.Reasoning`, thinking events | Request a neutral effort level and render thinking separately | [Reasoning output](recipes/reasoning.md) |
| Define structured tools | `NewTool`, `MustTool` | Derive tool JSON Schema from a Go struct | [Tools](tools.md) |
| Decode tool calls | `DecodeToolCall`, `ValidateToolCall` | Coerce, validate, and decode model arguments | [Validation](tools.md#validate-before-executing) |
| Run a tool loop | `ToolCalls`, `ToolResult`, `StopReasonToolUse` | Execute tools in application code and append results | [Tool loop](recipes/tool-loop.md) |
| Persist conversations | `json.Marshal(Context)`, `MarshalMessage` | Store messages and content blocks as self-describing JSON | [Conversation persistence](recipes/conversation-persistence.md) |
| Read token usage and cost | `AssistantMessage.Usage`, `CalculateCost` | Read input, output, cache tokens, and catalog-priced estimates | [Results](results.md) |
| Observe or rewrite requests | `OnRequest`, `RewriteRequest`, `OnResponse` | Trace every SDK attempt or patch provider-specific JSON | [Observability hooks](recipes/observability.md) |
| Detect context overflow | `IsContextOverflow` | Inspect errors, stop reason, and usage | [Error handling](recipes/error-handling.md) |
| Build a model picker | `GetProviders`, `GetRunnableModels` | Show models whose protocol is registered in this process | [Model discovery](recipes/provider-discovery.md) |
| Inspect provider credentials | `AuthStatus`, `APIKeyEnvVars` | Check key source and missing variables without a request | [Auth discovery](recipes/provider-discovery.md) |
| Route through a proxy | `ProviderRegistry.SetOverride` | Override URL, key, headers, and environment per provider | [Custom gateway](recipes/custom-gateway.md) |
| Use a compatible endpoint | Construct `Model` | Connect to OpenAI Chat Completions or Anthropic Messages compatibility endpoints | [Custom gateway](recipes/custom-gateway.md) |
| Inject an HTTP client | `openai.NewAdapter`, `anthropic.NewAdapter` | Configure transport, proxy, TLS, pools, or mocks | [Explicit client](recipes/custom-client.md) |
| Isolate global state | `NewClient`, `NewAdapterRegistry` | Build independent clients for tests, tenants, or subsystems | [Explicit client](recipes/custom-client.md) |
| Add a provider | `NewSpecProvider`, `ProviderRegistry.Register` | Register credential sources, headers, and models | [Custom providers](providers.md#register-a-custom-provider) |
| Add a wire protocol | `ProtocolAdapter`, `StreamWriter` | Translate requests and emit normalized stream events | [Custom protocols](extending.md) |
| Test without a real API | Construct `AssistantMessage`, use `httptest.Server` | Test result handling or the complete protocol path | [Mock-provider testing](recipes/mock-testing.md) |

## Minimal path

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		log.Fatal("model is not runnable")
	}

	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels in one sentence."),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}
```

Set `DEEPSEEK_API_KEY`, then run:

```sh
go run .
```

## Boundaries

`llm` does not currently provide:

- a conversation database or automatic history management;
- context summarization, trimming, or compaction;
- a tool executor, tool authorization, or automatic tool loop;
- agent planning, task scheduling, or a run state machine;
- RAG, a vector database, or document indexing;
- provider fallback, load balancing, or model racing;
- OpenAI Responses, Google Generative AI, or Mistral Conversations adapters.

Applications may build these features above `llm`. The current project material does not define built-in implementations.

## Choosing an entry point

- Use `Complete` when the caller only needs the final message.
- Use `Stream` for time-to-first-token or incremental rendering, and consume the event channel until it closes.
- Use an explicit `Client` for an isolated HTTP client, registry, or test environment.
- Construct a `Model` or register a provider when the endpoint remains wire-compatible.
- Implement `ProtocolAdapter` only for a different wire protocol.
