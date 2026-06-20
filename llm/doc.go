// Package llm is a unified, provider-neutral API for large language models.
//
// It speaks two wire protocols, OpenAI Chat Completions and Anthropic Messages,
// behind one set of types. The same conversation can be sent to any model on
// either protocol, and the target model can change between turns; the library
// re-adapts the history for each request. It is a stateless translation layer:
// it decides what to send for a request and how to interpret the streamed
// response, and leaves history storage, context compaction, and tool-loop
// orchestration to the caller.
//
// # Entry points
//
// Stream returns a channel of [Event] values; Complete consumes that stream and
// returns the final [AssistantMessage]. Both dispatch to the adapter registered
// for the model's protocol. The package keeps a default client; NewClient
// returns an isolated one with the built-in adapters registered.
//
//	model := llm.GetModel("anthropic", "claude-opus-4-8")
//	msg, err := llm.Complete(ctx, model, input, llm.StreamOptions{})
//
// # Building a request
//
// A [Context] holds the system prompt, the message history, and the available
// tools. Messages are provider-neutral and serialize to self-describing JSON, so
// a conversation can be persisted and replayed later against any model:
//
//   - [UserMessage], [AssistantMessage], [ToolResultMessage]
//   - content blocks: [TextContent], [ThinkingContent], [ImageContent], [ToolCall]
//
// Text-only models that receive image content have it downgraded to a
// placeholder automatically.
//
// # Options
//
// [StreamOptions] carries settings shared by every protocol — API key,
// temperature, max tokens, headers, retries, timeout — plus two observation
// hooks, OnRequest (the exact serialized request body) and OnResponse (status
// and headers), each invoked once per attempt including retries.
//
// Reasoning is a provider-neutral effort level ([ModelThinkingLevel]: off,
// minimal, low, medium, high, xhigh). Each adapter maps it to that provider's
// native form (Anthropic adaptive or budget thinking; the OpenAI-compatible
// reasoning fields) and clamps it to what the model supports. It is ignored by
// non-reasoning models.
//
// Settings with no neutral equivalent live on a protocol-specific extension
// supplied through StreamOptions.ProtocolOptions and validated against the target
// protocol before the request is sent:
//
//   - [AnthropicStreamOptions]: ThinkingDisplay (summarized or omitted) and a
//     native Anthropic ToolChoice ([AnthropicToolChoiceAuto], [AnthropicToolChoiceAny],
//     [AnthropicToolChoiceNone], or [AnthropicToolChoiceTool]).
//   - [OpenAICompletionsStreamOptions]: a native OpenAI ToolChoice
//     ([OpenAIToolChoiceAuto], [OpenAIToolChoiceNone], [OpenAIToolChoiceRequired],
//     or [OpenAIToolChoiceFunction]).
//
// # Streaming
//
// [Event] values report incremental progress: text, reasoning, and tool-call
// blocks each emit start, delta, and end events, followed by a single terminal
// EventDone (carrying the final message) or EventError. Every event carries a
// Partial snapshot of the message assembled so far. Tool-call arguments are
// parsed best-effort at end of stream; validate them and wait for EventDone
// before executing any call.
//
// # Typed tools
//
// NewTool and MustTool derive a provider-compatible JSON Schema from a Go struct,
// and DecodeToolCall decodes a returned call back into that struct. Malformed or
// truncated argument JSON degrades to a best-effort value and is reported in
// [AssistantMessage].Diagnostics rather than failing the response.
//
// # Results
//
// [AssistantMessage] holds the response content, a [StopReason], token [Usage]
// with per-category cost, and any non-fatal Diagnostics. CalculateCost prices a
// usage record against a model.
//
// # Switching models
//
// TransformMessages adapts a stored history for a target model before replay:
// it downgrades unsupported images, preserves reasoning signatures for the same
// model while downgrading or dropping them across models, normalizes tool-call
// identifiers, and repairs unanswered tool calls. Stream and Complete apply it
// automatically. IsContextOverflow reports whether a response exceeded the
// model's context window.
//
// # Models
//
// LookupModel and GetModel resolve models from the built-in catalog;
// GetProviders and GetModels enumerate it. SupportedThinkingLevels and
// ClampThinkingLevel report and adjust a model's reasoning levels. A caller may
// also construct a [Model] directly, pointing BaseURL at any OpenAI-compatible or
// Anthropic-compatible endpoint.
//
// # Custom protocols
//
// A genuinely different wire protocol is added by implementing [ProtocolAdapter]
// and registering it: build a [Registry] with NewRegistry, add the built-ins
// with RegisterBuiltins and the custom adapter with Registry.Register, then
// construct a client with NewClientWithRegistry. NewStreamWriter gives the
// adapter the same event-stream machinery the built-ins use.
package llm
