# Examples index

This page indexes runnable programs under `example/llm/` in the repository. For shorter task-oriented examples, use the [LLM recipes](recipes/README.md).

## Running an example

Each directory is a separate `main` package using the root module:

```sh
go run ./example/llm/basic
```

Live requests require the selected provider's API key. Check [Protocol and provider status](support-matrix.md) first.

## Example programs

| Example | Source directory | Demonstrates | Documentation |
|---|---|---|---|
| Basic | `example/llm/basic` | `LookupModel`, `Complete`, text results | [Basic completion](recipes/basic-completion.md) |
| Options | `example/llm/options` | System prompt, temperature, max tokens | [Configuration](configuration.md) |
| Streaming | `example/llm/streaming` | Text deltas and terminal events | [Streaming responses](recipes/streaming-chat.md) |
| Reasoning | `example/llm/reasoning` | Reasoning effort and thinking events | [Requesting reasoning](recipes/reasoning.md) |
| Tools | `example/llm/tools` | Typed tools and an application tool loop | [Executing tool calls](recipes/tool-loop.md) |
| Conversation | `example/llm/conversation` | Caller-owned multi-turn history | [Messages and context](conversations.md) |
| Model switch | `example/llm/model_switch` | Changing protocol and model with one history | [Changing models in a conversation](recipes/model-switching.md) |
| Advanced | `example/llm/advanced` | Request hooks and low-level options | [Configuration](configuration.md) |
| Providers | `example/llm/providers` | Provider registration and overrides | [Providers](providers.md) |
| Who am I | `example/llm/whoami` | Provider credential status | [Provider status](providers.md#check-whether-a-provider-is-configured) |

## Compile check

This command compiles every tracked LLM example without sending a network request:

```sh
go test ./example/llm/...
```

Examples cover `llm` responsibilities only. A production application still owns session storage, tool authorization, retry policy, and context compaction.
