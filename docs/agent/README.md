# Agent package

`github.com/ktsoator/or/agent` turns a model into an autonomous multi-step actor.
It runs the tool-call loop — stream a turn, execute the tools the model requests,
append the results, and continue until the model stops — on top of the `or/llm`
package, while leaving history storage and context compaction to the caller.

It is a provider-neutral orchestration layer: a stateless engine (`RunLoop`) plus
an optional stateful wrapper (`Agent`), with extension points as function fields.
It does not bundle concrete tools, persistence, or a system prompt.

## Install

```sh
go get github.com/ktsoator/or/agent@latest
```

The examples below assume these imports:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)
```

## Defining a tool

A tool pairs an `llm.ToolDefinition` (a JSON schema derived from a Go struct) with
an `Execute` function. `Execute` reports failure by returning an error, which the
loop turns into an error result rather than aborting the run. The optional
`onUpdate` callback streams partial progress.

```go
type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to look up,minLength=1"`
}

weatherTool := agent.AgentTool{
	Definition: llm.MustTool[weatherArgs]("get_weather", "Get the current weather for a city"),
	Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
		var in weatherArgs
		if err := json.Unmarshal(args, &in); err != nil {
			return agent.ToolResult{}, err
		}
		onUpdate(agent.ToolResult{Details: "querying weather service"}) // optional progress
		return agent.ToolResult{
			Content: []llm.ToolResultContent{&llm.TextContent{Text: fmt.Sprintf("Sunny, 24°C in %s.", in.City)}},
		}, nil
	},
}
```

## Quick start

`New` builds a stateful agent; `Prompt` runs one task to completion and appends
the result to its transcript. `Prompt` accepts a string, a single `AgentMessage`,
or a slice. For a multimodal prompt, `agent.UserMessage` builds a user message
from text and images:

```go
assistant.Prompt(ctx, agent.UserMessage("What is in this picture?",
	llm.ImageContent{Data: base64PNG, MIMEType: "image/png"}))
```

```go
assistant := agent.New(agent.Options{
	SystemPrompt: "Call get_weather before answering a weather question.",
	Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
	Tools:        []agent.AgentTool{weatherTool},
})

if err := assistant.Prompt(context.Background(), "What is the weather in Shanghai?"); err != nil {
	log.Fatal(err)
}

// The transcript now holds the prompt, the tool call, its result, and the answer.
for _, message := range assistant.Snapshot().Messages {
	_ = message
}
```

## Observing a run

`Subscribe` registers a listener that receives every event in order. It returns a
function that removes the listener. `Prompt` blocks until the run finishes, so
listeners fire while it runs.

```go
unsubscribe := assistant.Subscribe(func(event agent.AgentEvent) {
	switch event.Type {
	case agent.ToolStart:
		fmt.Printf("\n[tool] %s %v\n", event.ToolName, event.Args)
	case agent.MessageUpdate:
		if event.LLMEvent != nil && event.LLMEvent.Type == llm.EventTextDelta {
			fmt.Print(event.LLMEvent.Delta) // stream the answer
		}
	}
})
defer unsubscribe()
```

The event types:

| Event | Meaning | Notable fields |
|---|---|---|
| `AgentStart` / `AgentEnd` | run boundaries | `AgentEnd.Messages` — everything the run appended |
| `TurnStart` / `TurnEnd` | one assistant response + its tools | `TurnEnd.ToolResults` |
| `MessageStart` / `MessageUpdate` / `MessageEnd` | a message entering, streaming, completing | `MessageUpdate.LLMEvent` — the underlying `llm.Event` |
| `ToolStart` / `ToolUpdate` / `ToolEnd` | one tool executing | `Args`, `Result`, `IsError` |

## The stateless engine

`RunLoop` is the engine under `Agent`. It takes the new prompts and a base
context, returns a channel of events, and leaves the transcript to you. The final
`AgentEnd` event carries the messages the run appended.

```go
events := agent.RunLoop(ctx,
	[]agent.AgentMessage{agent.FromLLM(&llm.UserMessage{
		Content: []llm.UserContent{&llm.TextContent{Text: "Weather in Shanghai?"}},
	})},
	agent.Context{Tools: []agent.AgentTool{weatherTool}},
	agent.LoopConfig{Model: llm.GetModel("deepseek", "deepseek-v4-flash")},
)

var appended []agent.AgentMessage
for event := range events {
	if event.Type == agent.AgentEnd {
		appended = event.Messages
	}
}
```

## Controlling tool calls

`BeforeToolCall` runs after arguments validate and before execution; returning
`block` skips the tool and uses `reason` as the error result.

```go
BeforeToolCall: func(c agent.BeforeToolCallCtx) (block bool, reason string) {
	if c.ToolCall.Name == "delete_file" {
		return true, "file deletion is disabled"
	}
	return false, ""
},
```

`AfterToolCall` runs after execution; a non-nil return overrides the result field
by field. Setting `Terminate` on every result in a batch stops the run after it.

```go
AfterToolCall: func(c agent.AfterToolCallCtx) *agent.AfterToolCallResult {
	stop := true
	return &agent.AfterToolCallResult{Terminate: &stop}
},
```

A batch runs **concurrently by default**. Force sequential execution for the whole
loop, or per tool:

```go
agent.New(agent.Options{ToolExecution: agent.ExecutionSequential /* ... */})

weatherTool.ExecutionMode = agent.ExecutionSequential // this tool forces its batch sequential
```

## Switching models between turns

`PrepareNextTurn` runs after each turn and may replace the model or thinking level
for the next one. Because history is re-adapted per request, the new model can
even speak a different wire protocol.

```go
PrepareNextTurn: func(c agent.TurnCtx) *agent.TurnUpdate {
	// Draft on a fast model, then review on a stronger one (different protocol).
	review := llm.GetModel("minimax-cn", "MiniMax-M3")
	return &agent.TurnUpdate{Model: &review}
},
```

## Stopping and resuming

`ShouldStopAfterTurn` requests a graceful stop before the next request.

```go
ShouldStopAfterTurn: func(c agent.TurnCtx) bool {
	return len(c.NewMessages) > 20 // guard against a runaway loop
},
```

`Continue` resumes from the current transcript without a new prompt — for retries,
or after appending messages out of band. A provider needs a user or tool result
as the latest turn, so when the transcript ends with an assistant message,
`Continue` falls back to queued messages: it drains the steering queue first, then
the follow-up queue, and runs whatever it finds. It errors only when the last
message is an assistant and both queues are empty.

```go
if err := assistant.Continue(ctx); err != nil {
	log.Fatal(err)
}
```

## Steering and follow-ups

`Steer` injects a message before the next turn; `FollowUp` injects one after the
agent would otherwise stop. Call them from another goroutine while `Prompt` runs.

```go
go func() {
	_ = assistant.Prompt(ctx, "Summarize the repository")
}()

assistant.Steer(agent.FromLLM(&llm.UserMessage{
	Content: []llm.UserContent{&llm.TextContent{Text: "Focus on the agent package."}},
}))
```

`SteeringMode` and `FollowUpMode` control how many queued messages drain at once:
`QueueAll` (default) injects all of them; `QueueOneAtATime` injects only the
oldest, leaving the rest for later turns.

```go
agent.New(agent.Options{SteeringMode: agent.QueueOneAtATime /* ... */})
```

## Dynamic API keys

`GetAPIKey` resolves the provider key before each turn, for short-lived tokens
that may expire during a long run.

```go
GetAPIKey: func(provider string) string {
	return currentOAuthToken(provider) // refreshed out of band
},
```

## Tuning requests

`StreamOptions` is the base set of per-request options passed to the model on
every turn — `Temperature`, `MaxTokens`, `Headers`, and the `OnRequest` /
`OnResponse` observers. The agent fills in `Reasoning` from `ThinkingLevel` and
`APIKey` from `GetAPIKey`, so those two fields are ignored.

```go
temperature := 0.2
assistant := agent.New(agent.Options{
	Model:         model,
	ThinkingLevel: llm.ModelThinkingHigh,
	StreamOptions: llm.StreamOptions{
		Temperature: &temperature,
		MaxTokens:   4096,
		OnRequest:   func(method, url string, body []byte) { log.Println(method, url) },
	},
})
```

## Custom messages

A run operates on `AgentMessage`: standard `llm` messages adapted with `FromLLM`,
plus application types that embed `Custom`. Custom messages stay in the transcript
and event stream but are dropped by the default `ConvertToLLM`, so they never
reach the model. Provide `ConvertToLLM` to project them yourself.

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

## Managing state

```go
state := assistant.Snapshot() // read-only snapshot, safe to call mid-run from another goroutine
// state.Messages grows as a run progresses; state.StreamingMessage holds the
// in-flight response, and state.PendingToolCalls lists executing tool calls.

assistant.HasQueuedMessages()  // any steering or follow-up queued?
assistant.ClearSteeringQueue() // drop queued steering messages
assistant.ClearQueues()        // drop both queues
assistant.Abort()              // cancel the current run
assistant.Reset()              // clear transcript, error, and queues; keep config
```

Runnable programs are in [`example/agent`](../../example/agent/README.md): `basic`
(one tool, one prompt) and `tool` (an interactive session with reasoning, tool
progress, and mid-session model switching).

## Scope

This package provides the orchestration mechanism and leaves policy to the
caller. Context compaction, session persistence, skills, and an execution
environment are deliberately out of scope for this layer; the `TransformContext`
hook is where compaction would later attach.

For exported types and functions, see the package documentation on
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/agent).
