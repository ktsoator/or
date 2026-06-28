# or

Choose the path from intent to action.

[![Go Reference](https://pkg.go.dev/badge/github.com/ktsoator/or/llm.svg)](https://pkg.go.dev/github.com/ktsoator/or/llm)
[![CI](https://github.com/ktsoator/or/actions/workflows/ci.yml/badge.svg)](https://github.com/ktsoator/or/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ktsoator/or)](https://goreportcard.com/report/github.com/ktsoator/or)
[![Release](https://img.shields.io/github/v/release/ktsoator/or)](https://github.com/ktsoator/or/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/ktsoator/or)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`or` is a modular Go toolkit for building applications with language models and
higher-level agents. A provider-neutral LLM package keeps conversations, tools,
reasoning, and streaming events stable while models and wire protocols change
underneath, and an agent package builds the tool-call loop, state, and streaming
events on top.

## Why `or`

- Use one conversation model across OpenAI-compatible and Anthropic-compatible
  providers.
- Stream text, reasoning, tool calls, usage, and errors through typed events.
- Define tools from Go structs and validate model-generated arguments.
- Preserve provider metadata needed for multi-turn reasoning and tool use.
- Switch models between turns without rebuilding conversation history.
- Add custom model protocols without expanding the shared request API.
- Run autonomous multi-step tool loops with streaming events, mid-run steering,
  and per-turn model switching.

## Packages

| Package | Status | Description |
|---|---|---|
| [`or/llm`](docs/llm/README.md) | Available | Unified model access, streaming, tools, reasoning, images, and conversation history |
| [`or/agent`](docs/agent/README.md) | Available | Stateful agent loop with tools, streaming events, steering, follow-ups, and abort |

Future packages can build higher-level orchestration on the same foundations
without turning the root package into a single large API.

## Requirements

- Go 1.24 or later
- An API key for the selected hosted provider, or a compatible local endpoint

## Install

Install the LLM package:

```sh
go get github.com/ktsoator/or/llm@latest
```

Set the API key expected by the selected provider. For example:

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

See [Providers and models](docs/llm/providers.md) for supported provider IDs,
environment variables, catalog discovery, and custom endpoints.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol (DeepSeek, Groq, xAI, ...)
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels briefly."),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}
```

Each protocol lives in a provider package that registers itself on import. Pull
in the protocols you use — and only their vendor SDKs — by importing the matching
provider package for its side effects (`llm/openai`, `llm/anthropic`), or import
`llm/all` for every built-in protocol at once.

Use `llm.Stream` instead of `llm.Complete` to consume deltas while the model is
generating:

```go
events, err := llm.Stream(ctx, model, input, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
for event := range events {
	switch event.Type {
	case llm.EventTextDelta:
		fmt.Print(event.Delta)
	case llm.EventError:
		log.Fatal(event.Err)
	}
}
```

Runnable examples for completions, streaming, tools, images, and model
switching are available in [`example/llm`](example/llm/README.md).

A runnable stateful Agent example is available in
[`example/agent`](example/agent/README.md).

## Documentation

Guides for both packages live at
**[ktsoator.github.io/or](https://ktsoator.github.io/or/)**.

API reference: [`or/llm`](https://pkg.go.dev/github.com/ktsoator/or/llm) ·
[`or/agent`](https://pkg.go.dev/github.com/ktsoator/or/agent)

## Supported protocols

The built-in adapters implement:

- OpenAI-compatible Chat Completions
- Anthropic-compatible Messages

The model catalog includes explicit compatibility metadata for DeepSeek,
MiniMax, Xiaomi MiMo, Z.AI, Moonshot AI, Kimi, Anthropic, OpenRouter, and other
compatible providers. Catalog presence is not a guarantee that every model has
been live-tested; both wire adapters are covered by automated mock-server tests.

## Project status

`v0.3.0` adds the `or/agent` package and is the recommended baseline for new
integrations. The project remains pre-1.0, so APIs may continue
to evolve between minor versions. Breaking changes will be called out in release
notes.

## Acknowledgements

This project is inspired by and partially adapted from
[earendil-works/pi](https://github.com/earendil-works/pi), created by Mario
Zechner.

## License

Released under the [MIT License](LICENSE).
