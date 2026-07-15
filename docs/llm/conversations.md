# Messages and context

This page defines the `Context`, message interfaces, content blocks,
constructors, and serialization contract. Complete implementations for
multi-turn conversations, images, persistence, and model switching live in the
corresponding [guides](recipes/README.md).

## Message and content model

A history is a `[]llm.Message`. `Message` is an interface with three
implementations, one per role. Each holds a slice of *content blocks*, and the
role constrains which block types are allowed:

| Message | Role | Allowed content blocks |
|---|---|---|
| `UserMessage` | user input | `TextContent`, `ImageContent` |
| `AssistantMessage` | model output | `TextContent`, `ThinkingContent`, `ToolCall` |
| `ToolResultMessage` | a tool's result | `TextContent`, `ImageContent` |

The content blocks are the leaf types you read and write:

| Block | Carries |
|---|---|
| `TextContent` | plain text (valid in any message) |
| `ImageContent` | base64 image data plus a MIME type |
| `ThinkingContent` | reasoning text and its provider signature (assistant only) |
| `ToolCall` | a tool name, an ID, and decoded arguments (assistant only) |

Because both the message and the blocks are typed, a stored conversation
round-trips through JSON without manual dispatch — see
[JSON serialization](#json-serialization).

For the common "just send text" case, reach for the convenience constructors
below. Build the struct literals by hand only when you need content a
constructor does not cover — for example mixing text and an image in one user
message, or seeding an assistant turn that carries a tool call. See
[Sending images](recipes/vision.md) for the complete image-input flow.

## Build messages

`Context`, `Message`, and the content blocks are fully general, but most calls
just send some text. Convenience constructors remove the nesting for that path:

```go
llm.Prompt("Explain Go channels briefly.")        // Context with one user text message
llm.PromptWithSystem("Be concise.", "Explain...") // ...with a system prompt
llm.UserText("hello")                             // *UserMessage
llm.AssistantText("hi there")                     // *AssistantMessage (seed history)
llm.UserImage(data, "image/png")                  // *UserMessage with one image
llm.ToolResult(callID, name, "result text")       // *ToolResultMessage
llm.NewContext(msg1, msg2, ...)                   // Context from messages
```

Read a response back with the matching accessors on `AssistantMessage`:

```go
response.Text()      // all text blocks joined
response.ToolCalls() // every tool call, in order
```

The longhand struct literals below remain valid; reach for them when you need
content a constructor does not cover, such as mixing text and images in one
message.

## History and model transformation

`llm` stores no session state. The caller owns a `[]llm.Message`, appends each
`*AssistantMessage` and the next user message, then passes the slice back in
`Context.Messages`. `SystemPrompt` belongs to the request context and is not
automatically inserted into the message slice. For concurrency, storage
boundaries, and a complete restore program, see
[Saving and restoring conversations](recipes/conversation-persistence.md).

Before a request, `TransformMessages` creates a target-model copy of history:

| Stored content | Transformation |
|---|---|
| Image sent to a text-only model | Replace it with a text placeholder |
| Reasoning from the same model | Preserve compatible thinking and signatures |
| Reasoning from another model | Remove model-service-private reasoning |
| Tool-call IDs | Normalize for the target protocol and update matching results |
| Failed or aborted assistant message | Remove it from the replay copy |
| Tool call without a result | Insert a synthetic error result |

“Same model” requires provider, protocol, and model ID to match. Transformation
does not mutate caller-owned history; unchanged message objects may be shared
with the source slice, so callers should treat input history as immutable.

See [Changing models in a conversation](recipes/model-switching.md) for the
complete cross-model flow and compatibility checks.

## JSON serialization

`Context` round-trips through JSON. Messages carry roles and content blocks
carry types, so unmarshalling restores concrete message and content
implementations. For stores that persist one message per record, use
`MarshalMessage` and `UnmarshalMessage`:

```go
data, err := llm.MarshalMessage(messages[0])
if err != nil {
	log.Fatal(err)
}

message, err := llm.UnmarshalMessage(data)
if err != nil {
	log.Fatal(err)
}
messages = append(messages, message)
```

`UnmarshalMessage` returns an error for an unknown role, unknown content type,
or malformed JSON. It does not silently coerce unsupported shapes. For file and
database examples, concurrent writes, and schema-version guidance, see
[Saving and restoring conversations](recipes/conversation-persistence.md). For
encoding, model-capability checks, and security boundaries around images, see
[Sending images](recipes/vision.md).

!!! warning "Serialized history is sensitive data"
    A serialized `Context` can contain user input, tool results (which may embed
    fetched documents or credentials), and provider reasoning signatures. Treat
    the JSON as sensitive: do not log it wholesale, and store or transmit it with
    the same care as the underlying data.
