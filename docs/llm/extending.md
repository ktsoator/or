# Custom protocols

The built-in adapters cover OpenAI-compatible and Anthropic-compatible
endpoints. To support a different wire protocol, implement `ProtocolAdapter`
and register it on a client.

An adapter implements two methods: `Protocol` returns its registry key, and
`Stream` translates the provider response into package events. `StreamWriter`
provides the same lifecycle machinery used by the built-in adapters: one
`EventStart`, a `Partial` snapshot on non-terminal events, exactly one terminal
event, and cancellation reported as `StopReasonAborted`.

**1. Declare the adapter and its registry key.** `Protocol` returns the key the
client uses to route models to this adapter.

```go
type myAdapter struct{ http *http.Client }

func (myAdapter) Protocol() llm.Protocol { return "my-protocol" }
```

**2. Set up the response message and a `StreamWriter`.** `Stream` returns the
channel immediately and does its work in a goroutine. `NewStreamWriter` emits
the opening `EventStart` and tracks `Partial` snapshots for you.

```go
events := make(chan llm.Event)
go func() {
	defer close(events)

	message := llm.AssistantMessage{
		Protocol: model.Protocol,
		Provider: model.Provider,
		Model:    model.ID,
	}
	writer := llm.NewStreamWriter(ctx, events, &message)
```

**3. Call the endpoint; report failures through the writer.** `writer.Fail`
emits the single terminal `EventError`, so a failed request still closes the
stream correctly.

```go
	reply, usage, err := callMyEndpoint(ctx, a.http, model, input, options)
	if err != nil {
		writer.Fail(err)
		return
	}
```

**4. Emit the content-block lifecycle.** Append the block to `message.Content`,
then emit a start event, a delta per chunk, and an end event with the final
text.

```go
	text := &llm.TextContent{}
	message.Content = append(message.Content, text)
	writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: 0})
	for chunk := range reply {
		text.Text += chunk
		writer.Emit(llm.Event{
			Type: llm.EventTextDelta, ContentIndex: 0, Delta: chunk,
		})
	}
	writer.Emit(llm.Event{
		Type: llm.EventTextEnd, ContentIndex: 0, Content: text.Text,
	})
```

**5. Record usage and stop reason, then finish.** `writer.Done` emits the
single terminal `EventDone` carrying the assembled message.

```go
	message.Usage = usage
	message.StopReason = llm.StopReasonStop
	writer.Done()
}()
return events, nil
```

<details>
<summary>Full adapter</summary>

```go
type myAdapter struct{ http *http.Client }

func (myAdapter) Protocol() llm.Protocol { return "my-protocol" }

func (a myAdapter) Stream(
	ctx context.Context,
	model llm.Model,
	input llm.Context,
	options llm.StreamOptions,
) (<-chan llm.Event, error) {
	events := make(chan llm.Event)
	go func() {
		defer close(events)

		message := llm.AssistantMessage{
			Protocol: model.Protocol,
			Provider: model.Provider,
			Model:    model.ID,
		}
		writer := llm.NewStreamWriter(ctx, events, &message)

		reply, usage, err := callMyEndpoint(ctx, a.http, model, input, options)
		if err != nil {
			writer.Fail(err)
			return
		}

		text := &llm.TextContent{}
		message.Content = append(message.Content, text)
		writer.Emit(llm.Event{Type: llm.EventTextStart, ContentIndex: 0})
		for chunk := range reply {
			text.Text += chunk
			writer.Emit(llm.Event{
				Type: llm.EventTextDelta, ContentIndex: 0, Delta: chunk,
			})
		}
		writer.Emit(llm.Event{
			Type: llm.EventTextEnd, ContentIndex: 0, Content: text.Text,
		})

		message.Usage = usage
		message.StopReason = llm.StopReasonStop
		writer.Done()
	}()
	return events, nil
}
```

</details>

Register it and build a client:

```go
registry := llm.NewAdapterRegistry()
if err := registry.Register(myAdapter{http: http.DefaultClient}); err != nil {
	log.Fatal(err)
}
// Pass nil for the provider registry to keep the environment API key lookup;
// pass llm.NewBuiltInProviderRegistry() to also apply provider overrides.
client := llm.NewClient(registry, nil)

model := llm.Model{
	ID: "x", Provider: "me", Protocol: "my-protocol", MaxTokens: 1024,
}
message, err := client.Complete(ctx, model, input, llm.StreamOptions{})
```

To serve the built-in protocols from the same client, also register
`openai.NewAdapter(nil)` and `anthropic.NewAdapter(nil)` (from
`github.com/ktsoator/or/llm/openai` and `github.com/ktsoator/or/llm/anthropic`)
into the registry.

The adapter owns translation in both directions: building the wire request,
framing the response, updating usage and stop reason, and emitting deltas.
`CloneToolCall` deep-copies tool calls for events. `ParseToolArgumentsMode`
provides the same incomplete-JSON recovery used by the built-in adapters.

## Custom protocol options

Settings with protocol-specific semantics can use the shared extension point
without changing `StreamOptions`:

```go
type myProtocolOptions struct {
	SafetyMode string
}

func (*myProtocolOptions) Protocol() llm.Protocol { return "my-protocol" }

func (options *myProtocolOptions) Validate(_ []llm.ToolDefinition) error {
	if options.SafetyMode == "" {
		return errors.New("safety mode is required")
	}
	return nil
}

options := llm.StreamOptions{
	ProtocolOptions: &myProtocolOptions{SafetyMode: "strict"},
}
```

`Client.Stream` verifies that `ProtocolOptions.Protocol()` matches the target
model, then calls `Validate` before invoking the adapter.

Do not create a new protocol adapter for a service that already implements
OpenAI Chat Completions or Anthropic Messages. Construct a `Model` with the
matching built-in protocol and endpoint compatibility instead. For explicit
wiring, custom transports, and isolated registries, see
[Clients and registries](clients-and-registries.md).
