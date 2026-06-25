# Tools

## Typed tools

Generate a provider-compatible JSON Schema from a Go struct instead of writing
tool parameters by hand. The same type validates, coerces, and decodes the tool
call returned by the model.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
)

type WeatherArgs struct {
	City  string `json:"city" jsonschema:"description=City name,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
	Days  int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func main() {
	ctx := context.Background()
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	weatherTool := llm.MustTool[WeatherArgs](
		"get_weather",
		"Get a weather forecast",
	)

	messages := []llm.Message{
		llm.UserText("What's the weather in Shanghai for the next 3 days?"),
	}
	input := llm.Context{
		Messages: messages,
		Tools:    []llm.ToolDefinition{weatherTool},
	}

	response, err := llm.Complete(ctx, model, input, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	messages = append(messages, &response)

	toolUsed := false
	for _, toolCall := range response.ToolCalls() {
		if toolCall.Name != weatherTool.Name {
			continue
		}

		arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, toolCall)
		if err != nil {
			log.Fatal(err)
		}
		result := fmt.Sprintf(
			"%s will be sunny for the next %d days (%s).",
			arguments.City,
			arguments.Days,
			arguments.Units,
		)
		messages = append(messages, llm.ToolResult(toolCall.ID, toolCall.Name, result))
		toolUsed = true
	}
	if !toolUsed {
		log.Fatal("model returned no weather tool call")
	}

	response, err = llm.Complete(ctx, model, llm.Context{
		Messages: messages,
		Tools:    []llm.ToolDefinition{weatherTool},
	}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())
}
```

Fields without `omitempty` are required. The generated schema is fully inline
and omits document metadata such as `$schema`, `$id`, `$ref`, and `$defs`.

Tool arguments streamed by a provider may be recovered from incomplete JSON.
See [stream diagnostics](streaming.md#tool-call-deltas-and-diagnostics) before
executing tools with side effects.

## Protocol-specific tool choice

Tool choice retains each protocol's native vocabulary. Supply it through
`ProtocolOptions`; the client validates that its type matches the selected
model protocol and that a named tool exists in the request context.

OpenAI-compatible Chat Completions uses `required` and function choices:

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
		ToolChoice: llm.OpenAIToolChoiceRequired,
		// To force one function instead:
		// ToolChoice: llm.OpenAIToolChoiceFunction{Name: "get_weather"},
	},
}
```

Anthropic Messages uses `any` and tool choices:

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ToolChoice: llm.AnthropicToolChoiceAny,
		// To force one tool instead:
		// ToolChoice: llm.AnthropicToolChoiceTool{Name: "get_weather"},
	},
}
```

Both protocols expose `Auto` and `None` constants. Any explicit tool choice
requires at least one tool in `Context.Tools`.
