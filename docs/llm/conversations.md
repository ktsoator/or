# Conversations

Conversation messages are provider-neutral. The same history can be persisted,
extended, and sent to another compatible model without rebuilding it.

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
compatible, downgrades or removes incompatible reasoning, and normalizes tool
call identifiers.

```go
ctx := context.Background()
draft := llm.GetModel("deepseek", "deepseek-v4-flash")
review := llm.GetModel("anthropic", "claude-opus-4-8")

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
