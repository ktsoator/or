# API reference

This page indexes the public symbols of `github.com/ktsoator/or/llm` by module.
It does not duplicate field defaults, event tables, or behavioral rules. Current
source and [pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) remain
authoritative for exact Go declarations; the linked reference page owns each
group's semantics.

## Request entry points

| API | Return | Purpose |
|---|---|---|
| `Complete(ctx, model, input, options)` | `(AssistantMessage, error)` | Collect a response and return the final or partial message |
| `Stream(ctx, model, input, options)` | `(<-chan Event, error)` | Receive normalized events while generation runs |
| `NewClient(adapters, providers)` | `*Client` | Create an independent client with explicit registries |
| `(*Client).Complete(...)` | `(AssistantMessage, error)` | Complete through a specified client |
| `(*Client).Stream(...)` | `(<-chan Event, error)` | Stream through a specified client |

See [Failure signals](errors.md) for preflight and streaming failures, and
[Getting started](getting-started.md) for the shortest request path.

## Inputs, messages, and history

| Category | Public symbols | Reference |
|---|---|---|
| Context constructors | `Prompt`, `PromptWithSystem`, `NewContext` | [Messages and context](conversations.md#build-messages) |
| Message constructors | `UserText`, `UserImage`, `AssistantText`, `ToolResult` | [Messages and context](conversations.md#build-messages) |
| Message interfaces | `Message`, `UserMessage`, `AssistantMessage`, `ToolResultMessage` | [Messages and context](conversations.md#message-and-content-model) |
| Content blocks | `TextContent`, `ImageContent`, `ThinkingContent`, `ToolCall` | [Messages and context](conversations.md#message-and-content-model) |
| Response access | `AssistantMessage.Text`, `AssistantMessage.ToolCalls` | [Responses and usage](results.md#content-and-metadata) |
| Serialization | `MarshalMessage`, `UnmarshalMessage` | [Messages and context](conversations.md#json-serialization) |
| History transformation | `TransformMessages` | [Messages and context](conversations.md#history-and-model-transformation) |

`NewAssistantMessage` initializes adapter output. `Context` and the concrete
message types implement JSON marshal and unmarshal.

## Streaming

| Category | Public symbols | Reference |
|---|---|---|
| Events | `Event`, `EventType`, `EventStart`, text/reasoning/tool-call events, `EventDone`, `EventError` | [Streaming events](streaming.md#event-reference) |
| Adapter output | `StreamWriter`, `NewStreamWriter` | [Custom protocols](extending.md) |
| Tool-call copying | `CloneToolCall` | [Streaming events](streaming.md#tool-call-deltas-and-diagnostics) |

Valid event fields, order, and termination rules are maintained only in
[Streaming events](streaming.md).

## Request options

| Category | Public symbols | Reference |
|---|---|---|
| Shared options | `StreamOptions`, `StreamOptions.Validate` | [Request options](configuration.md) |
| Protocol option interface | `ProtocolStreamOptions` | [Request options](configuration.md) |
| OpenAI options | `OpenAICompletionsStreamOptions` and `OpenAIToolChoice` types and constants | [Tool definitions and calls](tools.md#protocol-specific-tool-choice) |
| Anthropic options | `AnthropicStreamOptions` and `AnthropicToolChoice` types and constants | [Tool definitions and calls](tools.md#protocol-specific-tool-choice) |
| Thinking display | `ThinkingDisplay`, `ThinkingDisplaySummarized`, `ThinkingDisplayOmitted` | [Reasoning options](reasoning.md#anthropic-thinking-display) |

See [Request options](configuration.md) for field defaults, credential
precedence, hooks, and request rewriting.

## Tools

| Category | Public symbols | Reference |
|---|---|---|
| Definition | `ToolDefinition`, `NewTool[T]`, `MustTool[T]` | [Tool definitions and calls](tools.md#typed-tools) |
| Reading and decoding | `ToolCall`, `DecodeToolCall[T]` | [Tool definitions and calls](tools.md#validate-before-executing) |
| Generic validation | `ValidateToolCall`, `ValidateToolArguments` | [Tool definitions and calls](tools.md#validate-before-executing) |
| Best-effort parsing | `ParseToolArguments`, `ParseToolArgumentsMode`, `ArgumentsMode` constants | [Tool definitions and calls](tools.md#validate-before-executing) |
| Diagnostics | `ToolArgumentsDiagnostic`, `DiagnosticToolArgumentsRecovered` | [Responses and usage](results.md#diagnostics) |

These APIs do not execute or authorize tools. See
[Executing tool calls](recipes/tool-loop.md) for the complete application flow.

## Models and providers

| Category | Public symbols | Reference |
|---|---|---|
| Built-in catalog | `LookupModel`, `GetModel`, `GetProviders`, `GetModels`, `GetRunnableModels`, `SupportsProtocol` | [Models and providers](providers.md#discover-models) |
| Model capability | `Model`, `ModelInput`, `ModelThinkingLevel`, `SupportedThinkingLevels`, `ClampThinkingLevel` | [Models and providers](providers.md#model-metadata), [Reasoning options](reasoning.md) |
| Cost estimation | `ModelCost`, `CalculateCost` | [Responses and usage](results.md#token-usage-and-cost) |
| Model registry | `ModelRegistry`, `NewModelRegistry` | [Clients and registries](clients-and-registries.md#modelregistry) |
| Provider definition | `Provider`, `ProviderSpec`, `NewSpecProvider` | [Models and providers](providers.md#register-a-custom-provider) |
| Provider overrides | `ProviderOverride`, `AuthStatus` | [Models and providers](providers.md#provider-configuration-and-status) |
| Provider registry | `ProviderRegistry`, `NewProviderRegistry`, `NewBuiltInProviderRegistry`, `DefaultProviderRegistry` | [Clients and registries](clients-and-registries.md#providerregistry) |
| Credential helpers | `APIKeyEnvVars`, `FindEnvAPIKeys`, `FindEnvAPIKeysWithEnv`, `GetEnvAPIKey`, `GetEnvAPIKeyWithEnv`, `MissingAPIKeyError` | [Request options](configuration.md#supply-credentials-per-request) |

Model fields, compatibility configuration, and provider override behavior are
maintained only in [Models and providers](providers.md). See
[Protocol and provider status](support-matrix.md) for live-support boundaries.

## Protocols and registries

| Category | Public symbols | Reference |
|---|---|---|
| Protocols | `Protocol`, `ProtocolOpenAICompletions`, `ProtocolAnthropicMessages` | [Protocol and provider status](support-matrix.md) |
| Compatibility | `ModelCompatibility`, `OpenAICompletionsCompatibility`, `AnthropicMessagesCompatibility` | [Models and providers](providers.md#custom-and-compatible-endpoints) |
| Adapter | `ProtocolAdapter` | [Custom protocols](extending.md) |
| Adapter registry | `AdapterRegistry`, `NewAdapterRegistry`, `Register` | [Clients and registries](clients-and-registries.md) |
| Built-in adapters | `openai.NewAdapter`, `anthropic.NewAdapter`, `llm/all` | [Getting started](getting-started.md#register-a-protocol-adapter) |

## Results, failures, and diagnostics

| Category | Public symbols | Reference |
|---|---|---|
| Stop reasons | `StopReason` and `StopReasonStop`, `Length`, `ToolUse`, `Error`, `Aborted` | [Responses and usage](results.md#stop-reasons) |
| Tokens and cost | `Usage`, `UsageCost`, `ModelCost` | [Responses and usage](results.md#token-usage-and-cost) |
| Context overflow | `IsContextOverflow`, `OverflowPatterns` | [Responses and usage](results.md#detect-context-overflow) |
| Diagnostics | `Diagnostic`, `DiagnosticToolArgumentsRecovered` | [Responses and usage](results.md#diagnostics) |

See [Failure signals](errors.md) for how returned errors, `EventError`, and
failed messages relate across request stages.
