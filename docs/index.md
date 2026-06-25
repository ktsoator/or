# or

Choose the path from intent to action — a modular Go toolkit for building
applications with language models and higher-level agents.

[Get started](llm/getting-started.md){ .md-button .md-button--primary }
[View on GitHub](https://github.com/ktsoator/or){ .md-button }

<div class="grid cards" markdown>

- __Provider-neutral__

    One conversation model across OpenAI-compatible and Anthropic-compatible
    providers. Switch models between turns without rebuilding history.

- __Typed streaming__

    Stream text, reasoning, tool calls, usage, and errors through typed events,
    each carrying a partial snapshot of the response.

- __Structured tools__

    Define tools from Go structs and validate model-generated arguments, with
    best-effort recovery for truncated streams.

- __Reasoning aware__

    A provider-neutral effort level maps onto each provider's native thinking,
    preserving the metadata needed for multi-turn continuity.

</div>

## Packages

| Package | Status | Description |
|---|---|---|
| [`or/llm`](llm/README.md) | Available | Unified model access, streaming, tools, reasoning, images, and conversation history |
| [`or/agent`](agent/README.md) | Available | Stateful agent loop with tools, streaming events, steering, follow-ups, and abort |

## Install

```sh
go get github.com/ktsoator/or/llm@latest
```

## A first request

```go
model := llm.GetModel("anthropic", "claude-opus-4-8")
msg, err := llm.Complete(ctx, model, llm.Prompt("Hello"), llm.StreamOptions{})
```

For exported types and functions, see the package documentation on
[pkg.go.dev](https://pkg.go.dev/github.com/ktsoator/or/llm).
