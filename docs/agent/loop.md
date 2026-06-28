# The run-loop engine

`RunLoop` is the stateless engine that drives the tool-call loop. `Agent` is a
thin stateful wrapper over it that adds a retained transcript, event subscription,
and the steering and follow-up queues. Reach for `RunLoop` directly when you want
to own the state yourself — your own persistence, your own event plumbing, or
integration into an existing run loop.

## Signature

```go
func RunLoop(ctx context.Context, prompts []AgentMessage, base Context, cfg LoopConfig) <-chan AgentEvent
```

- `prompts` are the new messages that start the run.
- `base` is the existing context they extend — system prompt, prior transcript,
  and tools.
- `cfg` configures the run; every extension point is a function field.

It returns a channel of events. **The caller must drain it until it closes.** The
final `AgentEnd` event carries the messages the run appended, which you fold into
your own transcript.

```go
events := agent.RunLoop(ctx,
	[]agent.AgentMessage{agent.FromLLM(llm.UserText("Weather in Shanghai?"))},
	agent.Context{
		SystemPrompt: "Call get_weather before answering.",
		Tools:        []agent.AgentTool{weatherTool},
	},
	agent.LoopConfig{Model: llm.GetModel("deepseek", "deepseek-v4-flash")},
)

var appended []agent.AgentMessage
for event := range events {
	switch event.Type {
	case agent.MessageUpdate:
		// render streaming output
	case agent.AgentEnd:
		appended = event.Messages // everything the run added
	}
}
```

## LoopConfig

`LoopConfig` is the full set of knobs. Given a `Model` and the default
`ConvertToLLM`, the zero config is a plain tool loop with no interception.

```go
type LoopConfig struct {
	Model         llm.Model
	StreamOptions llm.StreamOptions
	StreamFn      StreamFn
	ConvertToLLM  func([]AgentMessage) []llm.Message
	GetAPIKey     func(provider string) string
	ToolExecution ExecutionMode

	BeforeToolCall      func(BeforeToolCallCtx) (block bool, reason string)
	AfterToolCall       func(AfterToolCallCtx) *AfterToolCallResult
	ShouldStopAfterTurn func(TurnCtx) bool
	PrepareNextTurn     func(TurnCtx) *TurnUpdate
	TransformContext    func([]AgentMessage) []AgentMessage

	GetSteeringMessages func() []AgentMessage
	GetFollowUpMessages func() []AgentMessage
}
```

The hook fields behave exactly as on `agent.Options` — see
[Lifecycle hooks](hooks.md) and [Configuration](configuration.md).

## Steering and follow-ups without an Agent

`Agent` backs `Steer` and `FollowUp` with concurrency-safe queues. With `RunLoop`
you provide the source functions yourself:

- `GetSteeringMessages` is polled after each turn's tool calls finish; return
  messages to inject before the next turn.
- `GetFollowUpMessages` is polled when the run would otherwise stop; return
  messages to keep it going.

```go
agent.LoopConfig{
	Model: model,
	GetSteeringMessages: func() []agent.AgentMessage {
		return drainMyQueue() // your own source
	},
}
```

## Choosing between RunLoop and Agent

| | `RunLoop` | `Agent` |
|---|---|---|
| State | you own the transcript | retained internally |
| Events | drain the returned channel | `Subscribe` listeners |
| Steering / follow-ups | provide `Get*Messages` functions | `Steer` / `FollowUp` queues |
| Concurrency | up to you | one run at a time, methods are safe |

Most applications want `Agent`. Use `RunLoop` when its statefulness is in your
way — for example, when the transcript already lives in a database and the agent
should not keep a second copy.
