# Agent examples

Runnable examples for the stateful `agent.Agent`. They call a live model API and
may incur provider charges. Set the DeepSeek API key and run them from the
repository root:

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
go run ./example/agent/basic   # smallest: one tool, one prompt
go run ./example/agent/tool    # interactive terminal session
go run ./example/agent/hooks   # interception points (needs MINIMAX_CN_API_KEY too)
go run ./example/agent/harness # harness session + dynamic prompt demo
```

## basic

The smallest stateful agent: one tool and one prompt. It subscribes to the event
stream to print the answer as it streams and to show when the tool runs.

## tool

An interactive terminal with colored reasoning, answers, tool progress, and a
compact run summary. The same Agent retains the conversation across prompts.
All lifecycle events are handled; enter `/debug` to display low-level events.
The `/model <provider> <model-id>` command switches models while preserving the
entire conversation transcript.

Ask as many questions as you like:

```text
You › What is the weather in Shanghai?
You › What about Beijing?
You › /quit
```

Enter `/quit`, `quit`, or `exit` to end the session. Enter `/thinking` to show
or hide provider-supplied reasoning. Set `NO_COLOR=1` to disable ANSI colors.
Weather data is simulated, so the example needs no second API or service.

For example, switch from DeepSeek V4 Flash to V4 Pro after an answer:

```text
You › /model deepseek deepseek-v4-pro
↻ MODEL Switched to deepseek/deepseek-v4-pro · 2 messages preserved
```

## hooks

A non-interactive program showing the agent's interception points: `BeforeToolCall`
gates each call and blocks one tool, `AfterToolCall` annotates results,
`PrepareNextTurn` switches to a different model — across wire protocols — after the
tool turn, and `ShouldStopAfterTurn` guards against a runaway loop. It runs tools
on DeepSeek (OpenAI-compatible) and answers on MiniMax (Anthropic-compatible), so
it needs both `DEEPSEEK_API_KEY` and `MINIMAX_CN_API_KEY`.

## harness

A small example for `agent/harness`. It uses an in-memory `Session`, recreates
the harness over that same session to resume prior messages, and builds a fresh
system prompt from `TurnInfo` before each run. It uses DeepSeek V4 Flash, so set
`DEEPSEEK_API_KEY` before running it.
