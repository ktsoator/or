# Tool definitions and calls

A tool is a function you declare and let the model *ask you to call* — to fetch
data, run a calculation, or take an action the model cannot perform itself. The
library never executes anything: it turns your Go type into a schema the model
sees, hands back any calls the model makes, and lets you feed the results in. One
round-trip looks like this: **the model emits a tool call → you decode and run it
→ you send the result back → the model continues with that result in context.**
For example, to answer a weather question the model does not look it up itself —
it emits a `get_weather(city=…)` call, the caller runs it and sends the result
back, and the model answers from that.

This page defines the tool types, schema generation, argument validation, and
protocol options. For a complete request → execute → reply program and its
production policy, see [Executing tool calls](recipes/tool-loop.md).

## At a glance

| Task | API |
|---|---|
| Define a tool from a struct | `NewTool[T]` / `MustTool[T]` → `ToolDefinition` |
| Attach tools to a request | `Context.Tools` |
| Read the model's calls back | `AssistantMessage.ToolCalls()` → `[]ToolCall` |
| Decode a call's arguments | `DecodeToolCall[T]` |
| Return a result | `ToolResult(id, name, text)` → `ToolResultMessage` |
| Validate without a Go type | `ValidateToolCall` / `ValidateToolArguments` / `ParseToolArguments` |
| Force or restrict the choice | `StreamOptions.ProtocolOptions` |

A `ToolDefinition` is just `Name`, `Description`, and a `Parameters` JSON Schema.
A `ToolCall` the model returns carries an `ID`, a `Name`, and decoded
`Arguments`; the `ID` and `Name` are what you echo back in the `ToolResult`.

## Typed tools

Generate a provider-compatible JSON Schema from a Go struct instead of writing
tool parameters by hand. The same type validates, coerces, and decodes the tool
call returned by the model.

**1. Describe the arguments as a struct.** The `jsonschema` tags become schema
constraints. Fields without `omitempty` are required. The generated schema is
fully inline and omits document metadata such as `$schema`, `$id`, `$ref`, and
`$defs`.

```go
type WeatherArgs struct {
	City  string `json:"city" jsonschema:"description=City name,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
	Days  int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}
```

The `jsonschema` tag understands the constraints the library validates against
the model's returned arguments:

| Constraint | Tag | Applies to |
|---|---|---|
| Required | omit `omitempty` (add it to make the field optional) | any |
| Description | `description=...` | any |
| Enum | `enum=celsius,enum=fahrenheit` | string, number |
| Numeric range | `minimum=` · `maximum=` · `exclusiveMinimum=` · `exclusiveMaximum=` | number, integer |
| String length | `minLength=` · `maxLength=` | string |
| Pattern | `pattern=^[A-Z]` | string |
| Array length | `minItems=` · `maxItems=` | array |

Build the tool from the type, then attach the definition to `Context.Tools`:

```go
weatherTool := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast")

input.Tools = []llm.ToolDefinition{weatherTool}
```

`response.ToolCalls()` returns model calls in order. `DecodeToolCall` validates
against the tool schema before decoding into the requested Go type:

```go
arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, toolCall)
result := llm.ToolResult(toolCall.ID, toolCall.Name, resultText)
```

See [Executing tool calls](recipes/tool-loop.md) for the complete program from
declaration through multi-round execution. This page owns only the types,
schema, validation, and message-correspondence contract.

`MustTool` panics when the type cannot produce a valid schema, which suits
tools declared at startup. Use `NewTool`, which returns an error instead, when a
tool is built dynamically and a failure must be handled rather than crash.

## Call and result correspondence

`StopReasonToolUse` means the model is waiting for tool results. Every
`ToolCall` must have one corresponding `ToolResultMessage`, and the result's
`ToolCallID` and tool name must match the call. In history, the assistant
message comes first, followed by its tool results; only then can the next model
request be sent.

One turn may contain several calls, and the model may request more tools after
reading their results. Loop limits, dispatch, execution-failure feedback, and
stop policy belong to the application flow and are maintained only in
[Executing tool calls](recipes/tool-loop.md).

## Validate before executing

`DecodeToolCall` validates arguments against the tool schema and decodes them
into your struct in one step; it is the path most applications use. When you do
not have a Go type for the arguments, validate into a generic map instead:

- `ValidateToolCall(tools, call)` — find the matching tool by name, then
  validate and coerce; returns the arguments as `map[string]any`.
- `ValidateToolArguments(tool, call)` — validate against one known tool.
- `ParseToolArguments(raw)` — best-effort parse of raw argument JSON with no
  schema check; pair with `ParseToolArgumentsMode` to learn whether the JSON was
  strict, repaired, partial, or invalid.

Tool arguments streamed by a provider may be recovered from incomplete JSON.
A safe application declines `partial` and `invalid` arguments and returns a tool
error so the model can retry. See
[stream diagnostics](streaming.md#tool-call-deltas-and-diagnostics) before
executing tools with side effects.

## Protocol-specific tool choice

Tool choice retains each protocol's native vocabulary. Supply it through
`ProtocolOptions`; the client validates that its type matches the selected
model protocol and that a named tool exists in the request context.

OpenAI-compatible Chat Completions uses `required` and function choices:

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
		ToolChoice: llm.OpenAIToolChoiceRequired,
		// To force one function instead:
		// ToolChoice: llm.OpenAIToolChoiceFunction{Name: "get_weather"},
	},
}
```

Anthropic Messages uses `any` and tool choices:

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ToolChoice: llm.AnthropicToolChoiceAny,
		// To force one tool instead:
		// ToolChoice: llm.AnthropicToolChoiceTool{Name: "get_weather"},
	},
}
```

Both protocols expose `Auto` and `None` constants. Any explicit tool choice
requires at least one tool in `Context.Tools`.

## Execution boundary

`llm` returns tool calls but never executes them. The application or a separate
orchestration layer must own the request → execute → reply loop. Orchestration
lifecycle, state, and event APIs are outside the scope of this LLM reference.
