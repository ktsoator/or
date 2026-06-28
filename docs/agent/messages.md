# Messages and custom types

A run operates on `AgentMessage` values, not raw `llm` messages. This lets an
application keep its own UI-only entries in the transcript — notices, separators,
status banners — alongside the messages the model actually exchanges.

## Two kinds of message

`AgentMessage` is a sealed interface with two implementations:

- **Adapted `llm` messages.** `FromLLM` wraps a standard `llm.UserMessage`,
  `llm.AssistantMessage`, or `llm.ToolResultMessage`. This is the common path,
  since the agent package cannot add methods to types owned by `llm`.
- **Your own types.** A struct that embeds `agent.Custom` satisfies
  `AgentMessage` without referencing the interface's unexported marker.

```go
prompt := agent.FromLLM(llm.UserText("Refactor the parser."))
```

`agent.UserMessage` is a shortcut for the frequent text-plus-images case:

```go
msg := agent.UserMessage("What is in this picture?",
	llm.ImageContent{Data: base64PNG, MIMEType: "image/png"})
```

## UI-only messages

Embed `agent.Custom` to define a message that lives in the transcript and the
event stream but is not part of the model conversation:

```go
type Notice struct {
	agent.Custom
	Text string
}

assistant := agent.New(agent.Options{
	Model:    model,
	Messages: []agent.AgentMessage{Notice{Text: "session resumed"}}, // kept, not sent
})
```

A `Notice` appears in `Snapshot().Messages` and flows through `MessageStart` /
`MessageEnd` events, so your UI can render it — but the default projection drops it
before the model sees the conversation.

## Projecting to the model

`ConvertToLLM` projects the transcript into `llm.Message` values for one request.
The default unwraps `FromLLM` messages and drops every other `AgentMessage`, so
custom messages stay in history but never reach the model.

Provide your own `ConvertToLLM` to project custom messages yourself — for example,
to render a `Notice` as a system note the model should see:

```go
assistant := agent.New(agent.Options{
	Model: model,
	ConvertToLLM: func(messages []agent.AgentMessage) []llm.Message {
		out := make([]llm.Message, 0, len(messages))
		for _, m := range messages {
			switch v := m.(type) {
			case Notice:
				out = append(out, llm.UserText("[system] "+v.Text))
			default:
				if std, ok := agent.ToLLM(m); ok { // unwrap FromLLM messages
					out = append(out, std)
				}
			}
		}
		return out
	},
})
```

The projection runs at the request boundary on every turn, after
`TransformContext`, so it always sees the current transcript.

## Persisting a transcript

`FromLLM`-wrapped messages hold standard `llm` messages, which serialize to
self-describing JSON and can be replayed against any model. Custom messages are
your own types: to persist and restore them, give them a `type` discriminator and
register a decoder in your application's storage layer — the agent package keeps
no persistence of its own.
