# or
Choose the path from intent to action.

## Quick start

The `llm` package includes the built-in OpenAI-compatible and Anthropic protocol
adapters and can call `Complete` directly:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-chat")
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Context{Messages: []llm.Message{
			&llm.UserMessage{Content: []llm.UserContent{
				&llm.TextContent{Text: "Explain Go channels briefly."},
			}},
		}},
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	for _, block := range response.Content {
		if text, ok := block.(*llm.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
}
```

Set the provider API key in the environment, for example
`DEEPSEEK_API_KEY`. Use `llm.NewClient` when an isolated built-in client is
needed.

## Typed tools

Generate a provider-compatible JSON Schema from a Go struct instead of writing
tool parameters by hand. The same type is used to validate, coerce, and decode
the tool call returned by the model:

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
	model := llm.GetModel("deepseek", "deepseek-chat")
	weatherTool := llm.MustTool[WeatherArgs](
		"get_weather",
		"Get a weather forecast",
	)

	messages := []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{
			&llm.TextContent{Text: "What's the weather in Shanghai for the next 3 days?"},
		}},
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
	for _, content := range response.Content {
		toolCall, ok := content.(*llm.ToolCall)
		if !ok || toolCall.Name != weatherTool.Name {
			continue
		}

		arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, *toolCall)
		if err != nil {
			log.Fatal(err)
		}
		result := getWeather(arguments)
		messages = append(messages, &llm.ToolResultMessage{
			ToolCallID: toolCall.ID,
			ToolName:   toolCall.Name,
			Content: []llm.ToolResultContent{
				&llm.TextContent{Text: result},
			},
		})
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
	for _, content := range response.Content {
		if text, ok := content.(*llm.TextContent); ok {
			fmt.Println(text.Text)
		}
	}
}

func getWeather(arguments WeatherArgs) string {
	units := arguments.Units
	if units == "" {
		units = "celsius"
	}
	return fmt.Sprintf(
		"%s will be sunny for the next %d days, around 24 degrees %s.",
		arguments.City,
		arguments.Days,
		units,
	)
}
```

Fields without `omitempty` are required. The generated schema is fully inline
and omits document metadata such as `$schema`, `$id`, `$ref`, and `$defs`.
