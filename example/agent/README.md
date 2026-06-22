# Agent examples

Runnable examples for the stateful `agent.Agent`. Both call a live model API and
may incur provider charges. Set the DeepSeek API key and run them from the
repository root:

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
go run ./example/agent/basic   # smallest: one tool, one prompt
go run ./example/agent/tool    # interactive terminal session
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
