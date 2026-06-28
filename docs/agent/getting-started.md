# Getting started

## Install

Create a Go application and install the agent package:

```sh
mkdir myagent
cd myagent
go mod init myagent
go get github.com/ktsoator/or/agent@latest
```

The agent drives the model through the `or/llm` package, so it needs a protocol
adapter registered. Import the provider package for the protocol your model
speaks — usually as a blank import — and set the API key the provider expects:

```sh
export DEEPSEEK_API_KEY=your-deepseek-api-key
```

```go
import (
	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol (DeepSeek, Groq, xAI, ...)
)
```

DeepSeek, Groq, xAI, and the other OpenAI-compatible vendors use `llm/openai`;
Anthropic and MiniMax use `llm/anthropic`. Import `llm/all` to register every
built-in protocol at once. See
[Providers and models](../llm/providers.md) for the full mapping.

## Your first agent

`agent.New` builds a stateful agent; `Prompt` runs one task to completion and
appends the result to the agent's transcript. A run is the full tool-call loop,
not a single model call: the agent streams an assistant turn, executes any tools
the model requested, appends the results, and continues until the model stops.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to look up,minLength=1"`
}

func main() {
	weather := agent.AgentTool{
		Definition: llm.MustTool[weatherArgs]("get_weather", "Get the current weather for a city"),
		Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
			var in weatherArgs
			if err := json.Unmarshal(args, &in); err != nil {
				return agent.ToolResult{}, err
			}
			return agent.ToolResult{
				Content: []llm.ToolResultContent{&llm.TextContent{Text: fmt.Sprintf("Sunny, 24°C in %s.", in.City)}},
			}, nil
		},
	}

	assistant := agent.New(agent.Options{
		SystemPrompt: "Call get_weather before answering a weather question.",
		Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
		Tools:        []agent.AgentTool{weather},
	})

	if err := assistant.Prompt(context.Background(), "What is the weather in Shanghai?"); err != nil {
		log.Fatal(err)
	}

	fmt.Println(assistant.Snapshot().Messages) // prompt, tool call, tool result, answer
}
```

`Prompt` accepts a string, a single `AgentMessage`, or a slice. It blocks until
the run finishes and returns an error only when the run ended in failure or
cancellation — see [Reading the result](#reading-the-result).

## What a run produces

A run appends every message it generated to the transcript, in order: the user
prompt, each assistant turn, and each tool result. `Snapshot` returns a read-only
copy of the current state, including the full transcript:

```go
for _, message := range assistant.Snapshot().Messages {
	_ = message // user message, assistant message, or tool result
}
```

The transcript is the agent's memory. The next `Prompt` extends it, so a
conversation accumulates across calls until you `Reset`.

## A multimodal prompt

`agent.UserMessage` builds a user message from text and optional images — the
text block first, then each image in order:

```go
assistant.Prompt(ctx, agent.UserMessage("What is in this picture?",
	llm.ImageContent{Data: base64PNG, MIMEType: "image/png"}))
```

Text-only models have image content downgraded to a placeholder automatically, so
the same prompt is safe to send to any model.

## Reading the result

Errors travel as messages: a failed turn becomes an assistant message with a
non-clean stop reason rather than a panic. `Prompt` surfaces that as a Go error,
and `Snapshot().ErrorMessage` holds the text of the most recent failure.

```go
if err := assistant.Prompt(ctx, "..."); err != nil {
	log.Printf("run failed: %v", err) // also in assistant.Snapshot().ErrorMessage
}
```

## Next steps

- Watch a run as it happens — text deltas, tool progress, turn boundaries — in
  [Events and state](events.md).
- Define richer tools, stream their progress, and control execution order in
  [Tools](tools.md).
- Inject messages mid-run or continue a stopped agent in
  [Steering and follow-ups](steering.md).
