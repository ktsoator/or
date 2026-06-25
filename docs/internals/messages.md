# Message types

[`conversation.go`](https://github.com/ktsoator/or/blob/main/internal/llm/conversation.go)
defines the conversation model: the types every adapter reads and writes. Nothing
here is tied to a provider. An adapter turns these neutral types into a provider's
wire format on the way out, and rebuilds the same types from the response stream
on the way in.

## Two levels

A [`Context`](#context) holds a list of messages; each message holds a list of
content blocks. Messages are the conversation turns; content blocks are the
payloads inside them.

```text
Context
└── Messages: []Message
        ├── UserMessage        → []UserContent
        ├── AssistantMessage   → []AssistantContent
        └── ToolResultMessage  → []ToolResultContent
```

## Closed sets built from marker interfaces

Go has no sum types, so the package uses interfaces with an unexported marker
method to fence off a fixed set of implementations:

```go
type Message interface {
	isMessage()
}

func (*UserMessage) isMessage()       {}
func (*AssistantMessage) isMessage()  {}
func (*ToolResultMessage) isMessage() {}
```

Because `isMessage` is unexported, no type outside this package can satisfy
`Message`. The set of message kinds is closed, and a type switch over it is
exhaustive in practice. The methods sit on pointer receivers, so the concrete
values flowing through the package are always `*UserMessage`, `*AssistantMessage`,
and `*ToolResultMessage`.

The same trick does double duty for content. A block declares which roles it may
appear in by implementing the matching role interface — and only that one:

```go
func (*TextContent) isUserContent()       {}
func (*TextContent) isAssistantContent()  {}
func (*TextContent) isToolResultContent() {}

func (*ThinkingContent) isAssistantContent() {} // assistant only
```

## Placement rules

The role interfaces turn "which block goes where" into a compile-time rule. A
`ThinkingContent` does not implement `UserContent`, so putting one in a
`UserMessage` will not compile.

| Block | UserMessage | AssistantMessage | ToolResultMessage |
|---|:---:|:---:|:---:|
| `TextContent` | ✓ | ✓ | ✓ |
| `ImageContent` | ✓ | — | ✓ |
| `ThinkingContent` | — | ✓ | — |
| `ToolCall` | — | ✓ | — |

## The four content blocks

```go
type TextContent struct {
	Text          string `json:"text"`
	TextSignature string `json:"textSignature,omitempty"`
}

type ImageContent struct {
	Data     string `json:"data"`     // base64-encoded bytes
	MIMEType string `json:"mimeType"`
}

type ThinkingContent struct {
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}
```

A few fields carry weight beyond the obvious payload:

- `ToolCall.ID` is the correlation key. A `ToolResultMessage` answers a call by
  echoing it in `ToolCallID`, which is how a result is matched to its request
  across a turn.
- `ToolCall.Arguments` is a decoded JSON object (`map[string]any`), not a raw
  string. The streamed argument text is parsed — best-effort, so a truncated
  stream still yields a value — before it lands here.
- `ThinkingContent.Redacted` marks reasoning the provider returned in redacted
  form: the text is withheld, but the block is kept so the turn stays well-formed
  and its signature can be replayed.

!!! note "The signature fields"
    `TextSignature`, `ThinkingSignature`, and `ThoughtSignature` are opaque
    provider metadata. The package never reads their contents; it only stores
    them and replays them on later turns, so a provider can verify the
    continuity of its own reasoning and tool use across requests. See
    [Switching models](transform.md) for how they are preserved or dropped when
    the target model changes.

## Token usage and stop reasons

Two small value types travel on every assistant response. `Usage` counts tokens
by category and carries the calculated `UsageCost`; the categories line up with
[`ModelCost`](models.md#pricing) so cost is a per-category multiply:

```go
type Usage struct {
	Input, Output, CacheRead, CacheWrite, TotalTokens int64
	Cost UsageCost
}

type UsageCost struct {
	Input, Output, CacheRead, CacheWrite, Total float64
}
```

`StopReason` is a closed set that normalizes each provider's stop signal into one
neutral vocabulary:

| Value | Meaning |
|---|---|
| `stop` | normal completion |
| `length` | truncated by the output-token cap |
| `toolUse` | stopped so the caller can run tool calls |
| `error` | provider or runtime failure |
| `aborted` | the request was cancelled |

## The three messages

`UserMessage` and `ToolResultMessage` are small. A user message is just a content
list; a tool result adds the call ID and error flag that tie it back to its
`ToolCall`:

```go
type UserMessage struct {
	Content []UserContent
}

type ToolResultMessage struct {
	ToolCallID string
	ToolName   string
	Content    []ToolResultContent
	IsError    bool
}
```

`AssistantMessage` is the larger one — model output plus the response metadata an
adapter fills in:

```go
type AssistantMessage struct {
	Content []AssistantContent

	Protocol     Protocol     // wire protocol used
	Provider     string       // vendor key
	Model        string       // requested model ID
	Usage        Usage        // tokens and calculated cost
	StopReason   StopReason   // why generation stopped
	Diagnostics  []Diagnostic // non-fatal events, nil when clean
	Timestamp    int64        // Unix milliseconds
	// ... ResponseModel, ResponseID, ErrorMessage omitted
}
```

An adapter does not fill these fields from scratch. `NewAssistantMessage(model)`
seeds the provider-independent metadata — `Protocol`, `Provider`, `Model`, and
`Timestamp` — so an adapter starts from a half-built message and only appends
content and the response-specific fields.

## Reading a response

Two helpers walk `Content` so callers do not type-switch by hand. Both are
nil-safe and preserve block order:

```go
func (message *AssistantMessage) Text() string      // joins every text block
func (message *AssistantMessage) ToolCalls() []ToolCall // every tool call, in order
```

`Text()` skips thinking and tool-call blocks; `ToolCalls()` returns `nil` when
the model requested no tools, which reads naturally next to a `StopReason` of
`toolUse`. `ToolCalls()` returns values, not pointers — copies the caller can pass
to a tool runner without aliasing the message's own blocks.

## Context

A request is assembled from three fields:

```go
type Context struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
}
```

`ToolDefinition` keeps its parameter schema as raw JSON (`json.RawMessage`), so a
schema generated elsewhere is passed through untouched.

How these types serialize to self-describing JSON — and decode back without a
manual dispatch table — is covered in
[`messages.go`](https://github.com/ktsoator/or/blob/main/internal/llm/messages.go).
