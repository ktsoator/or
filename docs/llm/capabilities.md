# Capabilities

Use this page to decide whether `github.com/ktsoator/or/llm` covers a development
task and which entry point to read next. Complete programs live in the
[guides](recipes/README.md); type and field contracts live under API and
concepts.

`llm` is a stateless model-call layer. It converts application messages, tools,
and request options into a request accepted by the selected model service, then
normalizes the response as one message or a stream of events. The application
still owns conversation storage, tool execution, and business retry policy.

## Choose an entry point

| Current need | Use | Continue with |
|---|---|---|
| Read one result after generation ends | `Complete` | [One-shot text generation](recipes/basic-completion.md) |
| Display increments while text is generated | `Stream` | [Streaming responses](recipes/streaming-chat.md) |
| Isolate registries, network configuration, or test state | Explicit `Client` | [Creating a custom client](recipes/custom-client.md) |
| Connect a proxy or private service compatible with an existing protocol | `ProviderOverride` or an explicit `Model` | [Connecting a custom model service](recipes/custom-gateway.md) |
| Support request and response formats not implemented by the framework | `ProtocolAdapter` | [Custom protocols](extending.md) |

Choosing `Complete` or `Stream` changes how the application reads output, not
the shared message, tool, usage, or stop-reason model. Package functions use the
default registries; an explicit `Client` uses registries supplied by the
application.

## Generation and response handling

### Read a complete result

`Complete` consumes the underlying event stream and returns an
`AssistantMessage`. Applications can read text, content blocks, stop reason,
token usage, estimated cost, response IDs, and diagnostics.

Use it for batch processing, background work, and requests that do not display
generation progress. A returned error can accompany a partial message, so do
not discard the response solely because `err != nil`. See
[Responses and usage](results.md) and [Failure signals](errors.md).

### Handle a streamed result

`Stream` returns a normalized event channel. Text, reasoning, and tool
arguments have separate start, delta, and end events; the request terminates
with `EventDone` or `EventError`.

Use it for terminal output, chat interfaces, first-content latency, or tool-call
progress. The channel is unbuffered and must be consumed until close. See
[Streaming events](streaming.md) for the complete contract.

## Messages, images, and conversations

`Context` carries the system instruction, message history, and available tools
for one request. Messages use role-specific types and content blocks and can be
stored and restored through JSON.

| Capability | `llm` provides | Application owns |
|---|---|---|
| Multi-turn conversation | Typed messages, history transformation, and JSON serialization | Conversation IDs, database writes, concurrent turns, and context trimming |
| Image input | `ImageContent`, model input metadata, and text-only degradation | File loading, MIME validation, size limits, and authorization |
| Model changes | A target-specific copy of history | Target selection, credential checks, semantic evaluation, and fallback policy |
| History persistence | `Context`, `MarshalMessage`, and `UnmarshalMessage` | Storage versioning, encryption, retention, and tenant isolation |

See [Saving and restoring conversations](recipes/conversation-persistence.md),
[Sending images](recipes/vision.md), and
[Changing models in a conversation](recipes/model-switching.md) for complete
flows. Type and transformation contracts are in
[Messages and context](conversations.md).

## Reasoning and tool calls

### Request reasoning

`StreamOptions.Reasoning` expresses reasoning effort with shared levels, which
the adapter converts into options understood by the target service. Model
metadata determines supported levels. Visible reasoning returns through
separate events and `ThinkingContent`.

The application decides whether reasoning is displayed, stored, or hidden. See
[Reasoning options](reasoning.md) for levels, signatures, and conversation
continuity, and [Requesting reasoning](recipes/reasoning.md) for a complete
program.

### Execute tool calls

`NewTool` and `MustTool` derive a tool schema from a Go struct. After the model
returns a `ToolCall`, the application reads arguments with `DecodeToolCall` or
the validation helpers, performs the operation, and returns a `ToolResult`.

`llm` does not execute tools or provide authorization or an automatic loop.
The application must handle permissions, deadlines, idempotency, concurrency,
and loop bounds. See [Tool definitions and calls](tools.md) and
[Executing tool calls](recipes/tool-loop.md).

## Models and service integration

The built-in catalog provides model IDs, protocols, input capabilities, context
windows, output limits, and pricing metadata. `GetRunnableModels` returns only
models whose adapter is registered in the current process; `AuthStatus` checks
whether credentials can be resolved.

Developers can:

- read a built-in model with `LookupModel`;
- construct a `Model` for one compatible service;
- set gateway, credential, or header overrides with
  `ProviderRegistry.SetOverride`;
- register provider configuration with `NewSpecProvider`;
- inject an `http.Client` to control proxies, TLS, pools, and transport;
- implement `ProtocolAdapter` for a new request and response format.

See [Models and providers](providers.md) for model metadata and provider
configuration. Catalog presence is not a live compatibility guarantee. Verify
authentication, streaming, tools, and failure paths against every production
target.

## Request observation and testing

`OnRequest`, `RewriteRequest`, and `OnResponse` observe or modify each SDK
attempt. Callbacks run synchronously on the request path, and raw requests can
contain prompts, tool arguments, and other sensitive content. See
[Recording and rewriting requests](recipes/observability.md).

Business result handling can be tested with constructed `AssistantMessage`
values. Request serialization and event conversion can be tested with an
`httptest.Server`, without a live account. See [Testing strategy](testing.md)
and [Testing with a local mock server](recipes/mock-testing.md).

## Capabilities not included

`llm` does not provide:

- a conversation database or automatic history management;
- context summarization, trimming, or compaction;
- a tool executor, authorization system, or automatic tool loop;
- agent planning, task scheduling, or a run state machine;
- RAG, a vector database, or document indexing;
- provider fallback, load balancing, or model racing;
- adapters for protocols marked catalog-only in
  [Protocol and provider status](support-matrix.md).

The application or a higher-level package must implement these capabilities.
Use the [API reference](api-reference.md) to locate exported symbols.
