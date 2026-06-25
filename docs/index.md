# or

Choose the path from intent to action.

`or` is a modular Go toolkit for building applications with language models and
higher-level agents. A provider-neutral LLM package keeps conversations, tools,
reasoning, and streaming events stable while models and wire protocols change
underneath, and an agent package builds the tool-call loop, state, and streaming
events on top.

## Why `or`

- Use one conversation model across OpenAI-compatible and Anthropic-compatible providers.
- Stream text, reasoning, tool calls, usage, and errors through typed events.
- Define tools from Go structs and validate model-generated arguments.
- Preserve provider metadata needed for multi-turn reasoning and tool use.
- Switch models between turns without rebuilding conversation history.
- Add custom model protocols without expanding the shared request API.
- Run autonomous multi-step tool loops with streaming events, mid-run steering, and per-turn model switching.

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
