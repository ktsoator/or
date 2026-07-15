# API reference

This page indexes the public API of `github.com/ktsoator/or/llm` by use case. Current source and [pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm) remain authoritative for exact Go declarations.

Usage levels:

- **Common**: normal application request paths;
- **Configuration**: model selection, credentials, gateways, or explicit clients;
- **Extension**: custom providers or protocols;
- **Low-level**: adapter, diagnostic, or test helpers.

## Request entry points

| API | Level | Parameters | Return | Failure behavior |
|---|---|---|---|---|
| `Complete(ctx, model, input, options)` | Common | `context.Context`, `Model`, `Context`, `StreamOptions` | `(AssistantMessage, error)` | Setup errors return a zero message; stream failures can return a partial message with an error |
| `Stream(ctx, model, input, options)` | Common | Same | `(<-chan Event, error)` | Setup errors return immediately; runtime failures arrive as `EventError` |
| `NewClient(adapters, providers)` | Configuration | `*AdapterRegistry`, `*ProviderRegistry` | `*Client` | Requests fail when `adapters` is nil |
| `(*Client).Complete(...)` | Configuration | Same as package `Complete` | `(AssistantMessage, error)` | Uses the client's registries |
| `(*Client).Stream(...)` | Configuration | Same as package `Stream` | `(<-chan Event, error)` | Returns an error when the protocol is unregistered |

## Input constructors

| API | Return | Purpose |
|---|---|---|
| `Prompt(text)` | `Context` | One text user message |
| `PromptWithSystem(system, user)` | `Context` | System prompt plus one text user message |
| `NewContext(messages...)` | `Context` | Preserve messages in argument order |
| `UserText(text)` | `*UserMessage` | Text user message |
| `UserImage(data, mimeType)` | `*UserMessage` | One base64 image message |
| `AssistantText(text)` | `*AssistantMessage` | Seed history or tests with assistant text |
| `ToolResult(callID, toolName, text)` | `*ToolResultMessage` | Text tool result with `IsError=false` |
| `NewAssistantMessage(model)` | `AssistantMessage` | Initialize protocol, provider, model, and timestamp for adapter output |

## Message model

### `Context`

| Field | Type | Meaning |
|---|---|---|
| `SystemPrompt` | `string` | Request system prompt; not automatically stored in history |
| `Messages` | `[]Message` | User, assistant, and tool-result history |
| `Tools` | `[]ToolDefinition` | Tools available for this request |

`Context` implements `MarshalJSON` and `UnmarshalJSON`, rebuilding concrete message and content-block types during decoding.

### Message types

| Type | Allowed content |
|---|---|
| `UserMessage` | `TextContent`, `ImageContent` |
| `AssistantMessage` | `TextContent`, `ThinkingContent`, `ToolCall` |
| `ToolResultMessage` | `TextContent`, `ImageContent` |

`Message`, `UserContent`, `AssistantContent`, and `ToolResultContent` are sealed with unexported marker methods. External packages cannot add message or content-block implementations.

### Content blocks

| Type | Primary fields | Meaning |
|---|---|---|
| `TextContent` | `Text`, `TextSignature` | Text and an optional provider signature |
| `ImageContent` | `Data`, `MIMEType` | Base64 image |
| `ThinkingContent` | `Thinking`, `ThinkingSignature`, `Redacted` | Reasoning text or an encrypted/redacted reasoning block |
| `ToolCall` | `ID`, `Name`, `Arguments`, `ThoughtSignature` | A tool invocation requested by the model |

Concrete message and block types implement JSON marshal/unmarshal methods. Use `json.Marshal` and `json.Unmarshal`; applications normally do not call those methods directly.

### `AssistantMessage`

Important fields are `Content`, `Protocol`, `Provider`, `Model`, `ResponseModel`, `ResponseID`, `Usage`, `StopReason`, `ErrorMessage`, `Diagnostics`, and `Timestamp`.

| Method | Return | Meaning |
|---|---|---|
| `Text()` | `string` | Concatenate all text blocks in order |
| `ToolCalls()` | `[]ToolCall` | Return all tool calls in order |
| `MarshalJSON()` | `([]byte, error)` | Encode self-describing JSON |
| `UnmarshalJSON(data)` | `error` | Restore concrete content-block types |

## Message serialization and transformation

| API | Meaning |
|---|---|
| `MarshalMessage(message)` | Encode one `Message` with role and content discriminators |
| `UnmarshalMessage(data)` | Decode one message into its concrete type |
| `TransformMessages(messages, model, normalizer)` | Adapt history for a target model without mutating the caller's original slice |

Built-in adapters call `TransformMessages` automatically from `Stream` and `Complete`.

## Streaming events

### `EventType`

| Constant | Meaningful fields |
|---|---|
| `EventStart` | `Partial` |
| `EventTextStart` | `ContentIndex`, `Partial` |
| `EventTextDelta` | `ContentIndex`, `Delta`, `Partial` |
| `EventTextEnd` | `ContentIndex`, `Content`, `Partial` |
| `EventThinkingStart` | `ContentIndex`, `Partial` |
| `EventThinkingDelta` | `ContentIndex`, `Delta`, `Partial` |
| `EventThinkingEnd` | `ContentIndex`, `Content`, `Partial` |
| `EventToolCallStart` | `ContentIndex`, `ToolCall`, `Partial` |
| `EventToolCallDelta` | `ContentIndex`, `Delta`, `ToolCall`, `Partial` |
| `EventToolCallEnd` | `ContentIndex`, `ToolCall`, `Partial` |
| `EventDone` | `Message` |
| `EventError` | `Message`, `Err` |

Consume the event channel until it closes. Execute tools only after `EventDone`.

### `StreamWriter`

Adapter authors create a writer with `NewStreamWriter(ctx, events, output)`.

| Method | Meaning |
|---|---|
| `Start()` | Idempotently emit `EventStart` |
| `Emit(event)` | Emit a non-terminal event with a `Partial` snapshot |
| `Done()` | Emit the single `EventDone` |
| `Fail(err)` | Emit the single `EventError` |

`CloneToolCall` deep-copies a tool call's argument map.

## Request configuration

### `StreamOptions`

| Field | Type | Default behavior |
|---|---|---|
| `APIKey` | `string` | Resolve from provider override or environment when empty |
| `Env` | `ProviderEnv` | nil; request-scoped environment overrides |
| `Temperature` | `*float64` | nil; do not override provider default |
| `MaxTokens` | `int64` | zero; omitted for OpenAI, falls back to `Model.MaxTokens` for Anthropic |
| `Headers` | `map[string]string` | nil; override same-name model/provider headers |
| `Reasoning` | `ModelThinkingLevel` | empty; use model/provider default |
| `ProtocolOptions` | `ProtocolStreamOptions` | nil |
| `MaxRetries` | `*int` | nil; use SDK default |
| `Timeout` | `time.Duration` | zero; use SDK request default |
| `OnRequest` | callback | nil; called for every serialized attempt |
| `RewriteRequest` | callback | nil; called before each attempt is sent |
| `OnResponse` | callback | nil; called for each HTTP response |

`StreamOptions.Validate(protocol, tools)` validates protocol-specific options. `Client.Stream` calls it automatically.

### Protocol-specific options

`ProtocolStreamOptions` requires:

```go
Protocol() Protocol
Validate(tools []ToolDefinition) error
```

Built-in types:

| Type | Fields |
|---|---|
| `OpenAICompletionsStreamOptions` | `ToolChoice OpenAIToolChoice` |
| `AnthropicStreamOptions` | `ThinkingDisplay`, `ToolChoice AnthropicToolChoice` |

Tool-choice constants and values:

- `OpenAIToolChoiceAuto`, `OpenAIToolChoiceNone`, `OpenAIToolChoiceRequired`;
- `OpenAIToolChoiceFunction{Name: ...}`;
- `AnthropicToolChoiceAuto`, `AnthropicToolChoiceAny`, `AnthropicToolChoiceNone`;
- `AnthropicToolChoiceTool{Name: ...}`.

`OpenAIToolChoice`, `OpenAIToolChoiceMode`, `AnthropicToolChoice`, and `AnthropicToolChoiceMode` represent the sealed unions.

## Tools

| API | Parameters | Return | Failure behavior |
|---|---|---|---|
| `NewTool[T](name, description)` | Name and description; `T` is the argument struct | `(ToolDefinition, error)` | Invalid name or schema returns an error |
| `MustTool[T](name, description)` | Same | `ToolDefinition` | Panics when invalid; intended for startup declarations |
| `DecodeToolCall[T](tool, call)` | Definition and model call | `(T, error)` | Schema validation or JSON decoding can fail |
| `ValidateToolCall(tools, call)` | Tool list and call | `(map[string]any, error)` | Unknown tool or invalid arguments |
| `ValidateToolArguments(tool, call)` | Known tool and call | `(map[string]any, error)` | Arguments violate schema |
| `ParseToolArguments(raw)` | Raw JSON string | `map[string]any` | Returns recovered fields or an empty map |
| `ParseToolArgumentsMode(raw)` | Raw JSON string | `(map[string]any, ArgumentsMode)` | No error return; mode reports recovery quality |
| `ToolArgumentsDiagnostic(id, name, mode)` | Call identity and parse mode | `(Diagnostic, bool)` | bool is false for strict JSON |

`ArgumentsMode` constants are `ArgumentsStrict`, `ArgumentsRepaired`, `ArgumentsPartial`, and `ArgumentsInvalid`.

## Models

### Catalog functions

| API | Meaning |
|---|---|
| `LookupModel(provider, modelID)` | Return `(Model, bool)`; use for dynamic input |
| `GetModel(provider, modelID)` | Return `Model`; panic for an unknown entry |
| `GetProviders()` | Return built-in catalog provider IDs |
| `GetModels(provider)` | Return every catalog model, including unimplemented protocols |
| `GetRunnableModels(provider)` | Return models routable by the default adapter registry |
| `SupportsProtocol(protocol)` | Report default adapter registration |

### `Model`

Key fields are `ID`, `Name`, `Provider`, `Protocol`, `BaseURL`, `Headers`, `Reasoning`, `ThinkingLevelMap`, `Input`, `ContextWindow`, `MaxTokens`, `Cost`, and `Compatibility`.

`Model.UnmarshalJSON` restores a concrete compatibility type from `Protocol`. Compatibility-bearing decoding currently supports OpenAI Completions and Anthropic Messages.

### Reasoning, input, and cost

| Type or API | Meaning |
|---|---|
| `ModelInput` | Input modality; constants `Text` and `Image` |
| `ModelThinkingLevel` | `Off`, `Minimal`, `Low`, `Medium`, `High`, `XHigh` |
| `ThinkingDisplay` | `ThinkingDisplaySummarized`, `ThinkingDisplayOmitted` |
| `SupportedThinkingLevels(model)` | Return neutral reasoning levels accepted by the model |
| `ClampThinkingLevel(model, level)` | Clamp a request to the nearest supported level |
| `CalculateCost(model, usage)` | Price usage from per-million-token catalog rates |

### `ModelRegistry`

| API | Meaning |
|---|---|
| `NewModelRegistry()` | Create an empty registry |
| `Register(model)` | Validate and add or replace a model |
| `Get(provider, modelID)` | Return a defensive copy |
| `Providers()` | Return sorted provider IDs |
| `Models(provider)` | Return models ordered by ID |

## Providers and credentials

### Environment helpers

| API | Meaning |
|---|---|
| `APIKeyEnvVars(provider)` | Return checked variable names in precedence order |
| `FindEnvAPIKeys(provider)` | Return configured process variable names |
| `FindEnvAPIKeysWithEnv(provider, env)` | Include request-scoped `ProviderEnv` |
| `GetEnvAPIKey(provider)` | Return the first available key |
| `GetEnvAPIKeyWithEnv(provider, env)` | Prefer request-scoped environment values |
| `MissingAPIKeyError(provider)` | Build an error naming provider and expected variables |

### Provider types

`ProviderSpec` fields are `ID`, `Name`, `EnvKeys`, `Models`, and `Headers`.

`NewSpecProvider(spec)` returns an independent snapshot. `Provider` methods:

| Method | Return |
|---|---|
| `ID()` | `string` |
| `Name()` | `string` |
| `Models()` | defensive `[]Model` |
| `EnvKeys()` | defensive `[]string` |

`ProviderOverride` fields are `BaseURL`, `APIKey`, `Headers`, and `Env`.

`AuthStatus` fields are `Configured`, `Source`, `Label`, and `Missing`.

### `ProviderRegistry`

| API | Meaning |
|---|---|
| `NewProviderRegistry()` | Empty registry |
| `NewBuiltInProviderRegistry()` | Build providers from the catalog |
| `DefaultProviderRegistry()` | Instance used by the package client |
| `Register(provider)` | Add or replace a provider |
| `Get(providerID)` | Look up a provider |
| `Providers()` | Return providers sorted by ID |
| `SetOverride(providerID, override)` | Store an override snapshot |
| `ClearOverride(providerID)` | Remove an override |
| `ResolveRequest(model, options)` | Apply credential, URL, and header precedence |
| `AuthStatus(providerID, env)` | Return credential status and existence flag |

## Protocol and compatibility configuration

Protocol constants:

- `ProtocolOpenAICompletions`
- `ProtocolAnthropicMessages`

`ModelCompatibility` requires `Protocol() Protocol`. Concrete types:

### `OpenAICompletionsCompatibility`

Fields: `SupportsStore`, `SupportsDeveloperRole`, `SupportsReasoningEffort`, `MaxTokensField`, `SupportsStrictMode`, `RequiresReasoningContentOnAssistantMessages`, `RequiresThinkingAsText`, `ThinkingFormat`, and `ZAIToolStream`.

### `AnthropicMessagesCompatibility`

Fields: `SupportsTemperature`, `SupportsCacheControl`, `SupportsCacheControlTools`, `ForceAdaptiveThinking`, and `AllowEmptySignature`.

Except for `MaxTokensField` and `ThinkingFormat`, compatibility switches use pointers to distinguish unspecified from explicit false. Unspecified fields use adapter compatibility detection.

## Results, errors, and diagnostics

### Stop reason

`StopReason` constants are `StopReasonStop`, `StopReasonLength`, `StopReasonToolUse`, `StopReasonError`, and `StopReasonAborted`.

### Usage

`Usage` fields are `Input`, `Output`, `CacheRead`, `CacheWrite`, `TotalTokens`, and `Cost`.

`UsageCost` and `ModelCost` both split input, output, cache read, and cache write. `UsageCost` also contains `Total`.

### Overflow and diagnostics

| API | Meaning |
|---|---|
| `IsContextOverflow(message, contextWindow)` | Detect a context overflow from error text or usage |
| `OverflowPatterns()` | Return a defensive copy of internal match patterns |
| `Diagnostic` | `Type`, `Timestamp`, `Message`, `Details` |
| `DiagnosticToolArgumentsRecovered` | Tool-argument recovery diagnostic type |

## Adapter extension

`ProtocolAdapter`:

```go
type ProtocolAdapter interface {
	Protocol() Protocol
	Stream(context.Context, Model, Context, StreamOptions) (<-chan Event, error)
}
```

Registration API:

| API | Meaning |
|---|---|
| `NewAdapterRegistry()` | Create an empty adapter registry |
| `(*AdapterRegistry).Register(adapter)` | Add or replace an adapter for a protocol |
| `(*AdapterRegistry).Get(protocol)` | Look up an adapter |
| `Register(adapter)` | Register with the package-default adapter registry |

Built-in subpackages:

- `openai.NewAdapter(httpClient)`: OpenAI Chat Completions adapter;
- `anthropic.NewAdapter(httpClient)`: Anthropic Messages adapter;
- `llm/all`: side-effect import of every built-in adapter.
