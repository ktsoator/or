# `llm` — source map

English | [简体中文](README.zh.md)

A provider-neutral API for large language models. One set of types speaks two
wire protocols (OpenAI Chat Completions and Anthropic Messages); the same
conversation can be sent to any model on either protocol, re-adapted per request.
The package is a stateless translation layer — it decides what to send and how to
read the streamed response, and leaves history storage, compaction, and the
tool loop to the caller.

This document maps the source for people reading or extending the package. For
usage see the [package docs on pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm)
(the godoc in [`doc.go`](doc.go)) and the [guides](../docs/llm/README.md).

## The five blocks

The package is roughly five layers, listed here in the order they build on each
other. Reading them top to bottom is also a reasonable path through the source.

### 1. Domain types — the vocabulary

What a conversation, a model, and a streamed event *are*. Everything else is
built on these, so start here.

| File | Holds |
|---|---|
| [`message.go`](message.go) | `Message`, `UserMessage`/`AssistantMessage`/`ToolResultMessage`, content blocks (`TextContent`, `ThinkingContent`, `ImageContent`, `ToolCall`), `Context`, `ToolDefinition`, `Usage`, `StopReason` |
| [`model.go`](model.go) | `Model`, `Protocol`, `ModelThinkingLevel`, `ModelCost`, per-protocol compatibility, and single-model operations (`CalculateCost`, `SupportedThinkingLevels`, `ClampThinkingLevel`) |
| [`events.go`](events.go) | `Event` and `EventType` — the unit of a stream |

### 2. Entry & dispatch — how a request runs

The path from a call to a provider adapter. **The public entry points live here.**

| File | Holds |
|---|---|
| [`default.go`](default.go) | Package-level `Stream`/`Complete`/`Register` over a default client; documents the import-for-side-effects registration pattern. **Start reading here.** |
| [`client.go`](client.go) | `Client.Stream`/`Complete`: validate options, pick the adapter, inject the API key, consume the stream |
| [`adapters.go`](adapters.go) | `ProtocolAdapter` (the extension point providers implement) and `AdapterRegistry` |
| [`options.go`](options.go) | `StreamOptions`, protocol-specific extensions (`AnthropicStreamOptions`, `OpenAICompletionsStreamOptions`), native tool-choice types, and their validation |

### 3. Model catalog

Where the built-in models come from.

| File | Holds |
|---|---|
| [`model_registry.go`](model_registry.go) | `ModelRegistry` and the package-level `LookupModel`/`GetModel`/`GetProviders`/`GetModels` |
| [`catalog.go`](catalog.go) | `//go:embed` of the generated catalog and the `go:generate` directive (data produced by [`internal/genmodels`](internal/genmodels)) |

### 4. Tool calls

The lifecycle of a tool call, from definition to validated arguments.

| File | Holds |
|---|---|
| [`tools.go`](tools.go) | `NewTool`/`MustTool` (derive a JSON Schema from a Go struct) and `DecodeToolCall` |
| [`jsonparse.go`](jsonparse.go) | Best-effort parsing of the argument JSON a model streams (`ParseToolArguments`, `ArgumentsMode`) |
| [`validation.go`](validation.go) | `ValidateToolCall`/`ValidateToolArguments` — the thin validation entry point |
| [`jsonschema.go`](jsonschema.go) | The generic JSON-Schema coercion + validation engine that does validation's heavy lifting |
| [`diagnostics.go`](diagnostics.go) | `Diagnostic` and `ToolArgumentsDiagnostic` — recorded when arguments are repaired rather than parsed cleanly |

### 5. Codec & helpers — read on demand

Supporting machinery; none of it is needed to understand the main flow.

| File | Holds |
|---|---|
| [`message_json.go`](message_json.go) | JSON marshal/unmarshal for every message and content type (large, but single-purpose) |
| [`transform.go`](transform.go) | `TransformMessages`: adapts a stored history for a target model — downgrades unsupported images, reconciles reasoning across models, normalizes tool-call IDs, repairs orphaned tool calls |
| [`stream.go`](stream.go) | `StreamWriter`: the shared machinery an adapter uses to emit events with a single terminal guarantee |
| [`prompt.go`](prompt.go) | `Prompt`/`UserText`/`ToolResult` convenience constructors |
| [`keys.go`](keys.go) | API-key lookup from provider environment variables |
| [`overflow.go`](overflow.go) | `IsContextOverflow` context-window detection |
| [`jsonhelpers.go`](jsonhelpers.go) | JSON deep-copy and `isJSONNull` |

## Request flow

```
llm.Stream / llm.Complete            (default.go — package facade)
        │
        ▼
Client.Stream                        (client.go)
        │  validate StreamOptions, resolve API key
        ▼
AdapterRegistry.Get(model.Protocol)  (adapters.go)
        │
        ▼
ProtocolAdapter.Stream               (llm/openai, llm/anthropic)
        │  TransformMessages → serialize → HTTP → parse SSE
        ▼
StreamWriter emits []Event           (stream.go)
        │
        ▼
Complete consumes until EventDone / EventError → AssistantMessage
```

`Complete` is a thin consumer over `Stream`: it drains events and returns the
final message, or the error carried by `EventError`.

## Shortest path to understanding

`doc.go` → `message.go` + `model.go` → `default.go` → `client.go` →
`adapters.go`, then read one provider (`openai/`) to see how a protocol is
actually implemented. Blocks 1–2 cover the trunk; 3–5 are read on demand.

## Subpackages

| Package | Role |
|---|---|
| [`openai/`](openai) | The OpenAI Chat Completions adapter; registers itself on import |
| [`anthropic/`](anthropic) | The Anthropic Messages adapter; registers itself on import |
| [`all/`](all) | Blank-imports both providers to register every built-in protocol at once |
| [`internal/jsonx`](internal/jsonx) | Partial/lenient JSON parsing used by `jsonparse.go` |
| [`internal/genmodels`](internal/genmodels) | Generator for `catalog.generated.json` |

A provider package implements `ProtocolAdapter`, translates the neutral
`Message`/`StreamOptions` into its wire format, and calls `Register` from an
`init` function. Adding a genuinely new wire protocol means implementing that
interface and registering it — see the [extending guide](../docs/llm/extending.md).
