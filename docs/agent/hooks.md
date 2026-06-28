# Lifecycle hooks

Every extension point is a function field on `agent.Options` (or `LoopConfig` for
the stateless engine). The zero value of each is "no hook", so the bare agent is a
plain tool loop. The hooks let you intercept tool calls, switch models between
turns, stop early, and reshape the context before each request.

## When each hook fires

Within one turn, hooks fire in this order:

```
TransformContext        (just before the request is built)
  → the assistant turn streams
  for each tool call:
    BeforeToolCall       (after arguments validate, before execution)
      → Execute
    AfterToolCall        (after execution, before the result is emitted)
PrepareNextTurn          (after the turn and its tool results are appended)
ShouldStopAfterTurn      (before the next request starts)
```

## Intercepting tool calls

`BeforeToolCall` runs after the arguments validate and before the tool executes.
Return `block = true` to skip the tool; `reason` becomes the error result text the
model sees.

```go
BeforeToolCall: func(c agent.BeforeToolCallCtx) (block bool, reason string) {
	if c.ToolCall.Name == "delete_file" {
		return true, "file deletion is disabled in this session"
	}
	return false, ""
},
```

`AfterToolCall` runs after the tool finishes. A non-nil return overrides the
result field by field; a nil field keeps the executed value.

```go
AfterToolCall: func(c agent.AfterToolCallCtx) *agent.AfterToolCallResult {
	if c.IsError {
		stop := true
		return &agent.AfterToolCallResult{Terminate: &stop} // end the run on any tool error
	}
	return nil
},
```

`AfterToolCallResult` overrides `Content`, `Details`, `IsError`, and `Terminate`.
Setting `Terminate` on every result in a batch stops the run after it. Both hooks
run in source order and never concurrently, even when the tools themselves run in
parallel — see [Tools](tools.md#execution-order).

## Switching models between turns

`PrepareNextTurn` runs after each turn and may replace the model, the thinking
level, or the context for the next turn. Because history is re-adapted per request,
the new model can even speak a different wire protocol.

```go
PrepareNextTurn: func(c agent.TurnCtx) *agent.TurnUpdate {
	// Draft on a fast model, then review on a stronger one (different protocol).
	if len(c.NewMessages) == 2 {
		review := llm.GetModel("minimax-cn", "MiniMax-M3")
		return &agent.TurnUpdate{Model: &review}
	}
	return nil
},
```

`TurnUpdate` carries optional `Context`, `Model`, and `ThinkingLevel`; nil fields
keep the current value. `TurnCtx` gives the hook the turn's assistant message, its
tool results, the current context, and `NewMessages` — what the run would return
if it stopped now.

## Stopping early

`ShouldStopAfterTurn` requests a graceful stop before the next request starts.
The agent has no built-in turn cap, so this is where you guard against a runaway
loop.

```go
ShouldStopAfterTurn: func(c agent.TurnCtx) bool {
	return len(c.NewMessages) > 20 // cap the number of messages a run may add
},
```

## Reshaping the context

`TransformContext` adjusts the transcript before it is projected to `llm` messages
for each request. It is the attachment point for context compaction — summarizing
or dropping old turns to fit the window — which the package ships no default for.

```go
TransformContext: func(messages []agent.AgentMessage) []agent.AgentMessage {
	if len(messages) <= 40 {
		return messages
	}
	return compact(messages) // your summarization strategy
},
```

It runs per request and does not mutate the stored transcript, so compaction
affects only what the model sees, not the agent's memory.

## Safety

A panic from any hook is recovered into a terminal error event, so a misbehaving
callback ends the run cleanly instead of crashing the process. The same holds for
`ConvertToLLM` (see [Messages and custom types](messages.md)).
