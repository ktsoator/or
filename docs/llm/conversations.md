# Conversations

Conversation messages are provider-neutral. The same history can be persisted,
extended, and sent to another compatible model without rebuilding it.

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
[Save and restore](#save-and-restore-conversations).

For the common "just send text" case, reach for the convenience constructors
below. Build the struct literals by hand only when you need content a
constructor does not cover — for example mixing text and an image in one user
message (see [Image input](#image-input)), or seeding an assistant turn that
carries a tool call.

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

## Continue a conversation

A multi-turn conversation is a growing `[]llm.Message`. Append the assistant's
reply, then the next user message, and send the whole slice again. The library
is stateless, so the history you keep is the conversation.

```go
messages := []llm.Message{
	llm.UserText("Name a Go web framework."),
}

for _, turn := range []string{"And one for CLIs?", "Which is older?"} {
	reply, err := llm.Complete(ctx, model,
		llm.Context{Messages: messages}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	messages = append(messages, &reply)            // record the answer
	messages = append(messages, llm.UserText(turn)) // ask the follow-up
}
```

Append `&reply` (a pointer) so the assistant turn keeps the type the library
needs to replay it. A `SystemPrompt` set on the `Context` applies to every turn
without being stored in the message history.

## Image input

Multimodal models accept images alongside text in a user message. Provide the
raw bytes as base64 with their MIME type:

```go
raw, err := os.ReadFile("screenshot.png")
if err != nil {
	log.Fatal(err)
}
input := llm.Context{Messages: []llm.Message{
	&llm.UserMessage{Content: []llm.UserContent{
		&llm.TextContent{Text: "Describe the problem shown in this screenshot."},
		&llm.ImageContent{
			MIMEType: "image/png",
			Data:     base64.StdEncoding.EncodeToString(raw),
		},
	}},
}}
```

A model declares image support through `Model.Input`. When a history containing
images is sent to a text-only model, images are replaced with a short
placeholder automatically.

## Switch models between turns

Before each request, the library adapts stored history for the target model. It
downgrades images for text-only models, preserves reasoning signatures where
compatible, drops reasoning produced by another model, and normalizes tool
call identifiers.

The two models below even speak different wire protocols — DeepSeek is
OpenAI-compatible, MiniMax CN is Anthropic-compatible — yet the history slice is
reused as-is. Register both provider packages, since each protocol has its own
adapter:

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic" // MiniMax CN (Anthropic-compatible)
	_ "github.com/ktsoator/or/llm/openai"    // DeepSeek (OpenAI-compatible)
)

ctx := context.Background()
draft := llm.GetModel("deepseek", "deepseek-v4-flash")   // openai-completions
review := llm.GetModel("minimax-cn", "MiniMax-M2.7")      // anthropic-messages

messages := []llm.Message{
	llm.UserText("Compute 25 * 18 and explain the steps."),
}

first, err := llm.Complete(ctx, draft,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
messages = append(messages, &first)
messages = append(messages, llm.UserText("Check the calculation above for mistakes."))

second, err := llm.Complete(ctx, review,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
```

This needs `DEEPSEEK_API_KEY` and `MINIMAX_CN_API_KEY` in the environment. See
the runnable [`model_switch`](https://github.com/ktsoator/or/tree/main/example/llm/model_switch)
example for the complete program.

`TransformMessages` performs this adaptation and is exported for callers that
need to inspect the exact history a model would receive.

## Save and restore conversations

`Context` serializes to self-describing JSON: messages carry a role and content
blocks carry a type, so JSON round-trips into concrete message and content types
without manual dispatch.

```go
data, err := json.MarshalIndent(llm.Context{Messages: messages}, "", "  ")
if err != nil {
	log.Fatal(err)
}
if err := os.WriteFile("conversation.json", data, 0o644); err != nil {
	log.Fatal(err)
}

raw, err := os.ReadFile("conversation.json")
if err != nil {
	log.Fatal(err)
}
var restored llm.Context
if err := json.Unmarshal(raw, &restored); err != nil {
	log.Fatal(err)
}
```

`restored.Messages` is ready to extend and replay against any model.

For stores that persist one message per row or record, use `MarshalMessage` and
`UnmarshalMessage` instead of wrapping the value in a `Context`:

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
or malformed JSON. It does not silently coerce unsupported message shapes.

!!! warning "Serialized history is sensitive data"
    A serialized `Context` can contain user input, tool results (which may embed
    fetched documents or credentials), and provider reasoning signatures. Treat
    the JSON as sensitive: do not log it wholesale, and store or transmit it with
    the same care as the underlying data.
