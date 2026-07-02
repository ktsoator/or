# Agent package

`github.com/ktsoator/or/agent` turns a model into an autonomous multi-step actor.
It runs the tool-call loop — stream a turn, execute the tools the model requests,
append the results, and continue until the model stops — on top of the `or/llm`
package, while leaving history storage and context compaction to the caller.

It is a provider-neutral orchestration layer: a stateless engine (`RunLoop`) plus
an optional stateful wrapper (`Agent`), with every extension point a function
field. It bundles no concrete tools, persistence, or system prompt.

In other words, `llm` sends one model request; `agent` runs a task to completion.
When the model asks for a tool, the agent validates arguments, executes the tool,
appends the result to the transcript, and asks the model to continue. The run ends
only when the model produces a final answer, the context is canceled, or a hook
asks the loop to stop.

## When to use it

Use `agent` when your application needs to:

- Let a model think across multiple turns, call tools, read results, and continue.
- Keep user messages, assistant messages, and tool results in one transcript.
- Observe progress in real time: text deltas, tool starts/ends, tool updates, and turn boundaries.
- Inject steering while a run is active, queue follow-up work, or abort the agent.
- Control behavior with function hooks, such as blocking a tool call, replacing a result, switching models, or compacting context.

Use [`or/llm`](../llm/README.md) directly when you want to manage a single model
request yourself. Use
[`or/agent/harness`](https://pkg.go.dev/github.com/ktsoator/or/agent/harness)
on top of `agent` when you also want transcript persistence, automatic context
compaction, per-turn system prompts, skills, or prompt templates.

## What a run does

An `Agent.Prompt` run roughly follows this sequence:

1. Append the user input to the agent transcript.
2. Project the transcript to `llm.Message` values, call the current model, and stream the assistant message.
3. If the assistant requests tools, validate arguments against each tool schema and execute the matching `AgentTool`.
4. Append every tool result to the transcript so the model can read it.
5. Repeat until there are no more tool calls, or the run is canceled, blocked, or stopped.
6. After the run, `Snapshot().Messages` contains the full ordered sequence appended during the task.

The loop is not tied to any provider. If a model is exposed through `or/llm` as
the same message, tool, and streaming event shapes, `agent` can orchestrate it.

## Minimal example

Without tools, `Agent` is already a stateful model conversation with a retained
transcript:

```go
assistant := agent.New(agent.Options{
	SystemPrompt: "You are a concise Go tutor.",
	Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
})

if err := assistant.Prompt(ctx, "Explain goroutines in one sentence."); err != nil {
	log.Fatal(err)
}

messages := assistant.Snapshot().Messages
last, _ := agent.ToLLM(messages[len(messages)-1])
fmt.Println(last.(*llm.AssistantMessage).Text())
```

With a tool, the same `Prompt` becomes a complete tool-call loop:

```go
type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to look up,minLength=1"`
}

weather := agent.AgentTool{
	Definition: llm.MustTool[weatherArgs](
		"get_weather",
		"Get the current weather for a city",
	),
	Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
		var in weatherArgs
		if err := json.Unmarshal(args, &in); err != nil {
			return agent.ToolResult{}, err
		}
		return agent.ToolResult{
			Content: []llm.ToolResultContent{
				&llm.TextContent{Text: "Sunny, 24C in " + in.City},
			},
		}, nil
	},
}

assistant := agent.New(agent.Options{
	SystemPrompt: "Call get_weather before answering weather questions.",
	Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
	Tools:        []agent.AgentTool{weather},
})

err := assistant.Prompt(ctx, "What should I pack for Shanghai today?")
```

See [Getting started](getting-started.md) and the repository's
[`example/agent`](https://github.com/ktsoator/or/tree/main/example/agent) folder
for complete runnable programs.

## Two API layers

| Layer | Best for | Responsibility |
| --- | --- | --- |
| `Agent` | Most applications | Retain transcript, serialize prompts, emit events, support steer/follow-up/abort |
| `RunLoop` | Custom runtimes or existing state layers | Execute one stateless tool loop and return appended messages through events |

Start with `Agent` in most applications. Reach for `RunLoop` when your
transcript already lives in a database, queue, or runtime and you do not want the
library to retain another copy.

## What it does and does not do

`agent` handles:

- Tool-call looping and tool-result append.
- Stateful transcript and read-only snapshots.
- Streaming event subscriptions.
- Mid-run steering, follow-ups, and abort.
- Tool execution order, progress updates, interception, and turn-level hooks.
- Provider-neutral model switching, reasoning level, and dynamic API keys.

`agent` does not provide:

- Built-in search, filesystem, browser, or database tools.
- A default system prompt or safety policy.
- Cross-process transcript persistence.
- A default context compaction strategy.
- Job scheduling, service deployment, or user permission management.

Those boundaries are intentional: tools, storage, prompts, and permissions are
usually application-specific. The package gives you a composable run kernel and
leaves those policies in your application layer.

## Install

```sh
go get github.com/ktsoator/or/agent@latest
```

## Documentation

- [Getting started](getting-started.md) — your first agent and the tool loop
- [Tools](tools.md) — defining tools, results, streaming progress, and execution order
- [Events and state](events.md) — the run event stream, subscriptions, and snapshots
- [Steering and follow-ups](steering.md) — injecting messages mid-run, continuing, and aborting
- [Lifecycle hooks](hooks.md) — intercepting tools, switching models, stopping, and compaction
- [Messages and custom types](messages.md) — the transcript, UI-only messages, and projection
- [Configuration](configuration.md) — request options, reasoning, dynamic keys, and setters
- [The run-loop engine](loop.md) — `RunLoop`, `LoopConfig`, and building your own wrapper

For exported types and functions, see the package documentation on
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/agent).
