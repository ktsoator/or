# Events and state

A run emits a stream of events as it progresses, and the agent folds those events
into a live state you can read at any time. Together they let a UI render a run as
it happens — streaming text, tool progress, and turn boundaries — without polling.

## Subscribing

`Subscribe` registers a listener that receives every event in order and returns a
function that removes it. `Prompt` blocks until the run finishes, so listeners
fire while it runs; subscribe before calling `Prompt`.

```go
unsubscribe := assistant.Subscribe(func(event agent.AgentEvent) {
	switch event.Type {
	case agent.MessageUpdate:
		if event.LLMEvent != nil && event.LLMEvent.Type == llm.EventTextDelta {
			fmt.Print(event.LLMEvent.Delta) // stream the answer token by token
		}
	case agent.ToolStart:
		fmt.Printf("\n[tool] %s %v\n", event.ToolName, event.Args)
	case agent.ToolEnd:
		fmt.Printf("[done] %s (error=%v)\n", event.ToolName, event.IsError)
	}
})
defer unsubscribe()
```

Listeners run synchronously, in event order, on the goroutine driving the run. A
listener that blocks holds up the run, including tool execution, so keep them
fast — hand heavy work to another goroutine or a buffered channel.

## Event types

```go
type AgentEvent struct {
	Type        AgentEventType
	Message     AgentMessage         // the message a lifecycle event refers to
	LLMEvent    *llm.Event           // underlying llm event, set on MessageUpdate
	ToolResults []llm.ToolResultMessage // set on TurnEnd
	ToolCallID  string
	ToolName    string
	Args        any                  // validated tool arguments, on tool events
	Result      any                  // (partial) ToolResult, on tool events
	IsError     bool
	Messages    []AgentMessage       // appended messages, set on AgentEnd
}
```

Fields are populated according to `Type`; unrelated fields are zero.

| Event | Meaning | Notable fields |
|---|---|---|
| `AgentStart` / `AgentEnd` | run boundaries | `AgentEnd.Messages` — everything the run appended |
| `TurnStart` / `TurnEnd` | one assistant response and its tools | `TurnEnd.ToolResults` |
| `MessageStart` / `MessageUpdate` / `MessageEnd` | a message entering, streaming, completing | `MessageUpdate.LLMEvent` — the underlying `llm.Event` |
| `ToolStart` / `ToolUpdate` / `ToolEnd` | one tool executing | `ToolName`, `Args`, `Result`, `IsError` |

`MessageUpdate` carries the raw `llm.Event` in `LLMEvent`, so you can distinguish
text deltas from reasoning deltas and tool-call deltas, and read the partial
message assembled so far from `event.Message`.

## The lifecycle of a run

Events arrive in a predictable order:

```
AgentStart
  TurnStart
    MessageStart / MessageEnd        (the user prompt)
    MessageStart / MessageUpdate* / MessageEnd   (the assistant turn, streaming)
    ToolStart / ToolUpdate* / ToolEnd            (each tool the turn called)
    MessageStart / MessageEnd        (each tool result)
  TurnEnd
  ... another TurnStart while the model keeps calling tools ...
AgentEnd
```

A turn that calls no tools and leaves no steering messages ends the run after
`TurnEnd`. `AgentEnd.Messages` is the same slice the stateless `RunLoop` returns —
everything the run appended to the transcript.

## Reading state

`Snapshot` returns a read-only copy of the agent's current state. It is safe to
call from another goroutine while a run is in progress.

```go
type State struct {
	SystemPrompt     string
	Model            llm.Model
	ThinkingLevel    llm.ModelThinkingLevel
	Tools            []AgentTool
	Messages         []AgentMessage // grows as the run completes each message
	IsStreaming      bool           // a prompt or continuation is in progress
	StreamingMessage AgentMessage   // the in-flight response, or nil
	PendingToolCalls []string       // ids of tool calls currently executing
	ErrorMessage     string         // text of the most recent failed turn
}
```

The agent folds each event into this state before notifying listeners, so a
listener always observes the updated state:

- `Messages` grows as each message reaches `MessageEnd`.
- `StreamingMessage` tracks the response as deltas arrive and clears when it
  completes.
- `PendingToolCalls` lists the tool calls between their `ToolStart` and `ToolEnd`.

```go
state := assistant.Snapshot()
if state.IsStreaming {
	fmt.Print(state.StreamingMessage) // render the partial answer
}
```

## Next steps

- Inject messages while a run is streaming, or continue after it stops, in
  [Steering and follow-ups](steering.md).
- Intercept tool calls and switch models between turns in
  [Lifecycle hooks](hooks.md).
