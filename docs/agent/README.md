# Agent package

`github.com/ktsoator/or/agent` turns a model into an autonomous multi-step actor.
It runs the tool-call loop — stream a turn, execute the tools the model requests,
append the results, and continue until the model stops — on top of the `or/llm`
package, while leaving history storage and context compaction to the caller.

It is a provider-neutral orchestration layer: a stateless engine (`RunLoop`) plus
an optional stateful wrapper (`Agent`), with every extension point a function
field. It bundles no concrete tools, persistence, or system prompt.

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
