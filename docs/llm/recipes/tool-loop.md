# Tool loop

## Purpose

Let the model request structured tools, execute them in application code, and return results to the model.

## Program skeleton

```go
type WeatherArgs struct {
	City string `json:"city" jsonschema:"minLength=1"`
}

tool := llm.MustTool[WeatherArgs]("get_weather", "Get current weather")
messages := []llm.Message{llm.UserText("What's the weather in Paris?")}

for turn := 0; turn < 8; turn++ {
	response, err := llm.Complete(ctx, model, llm.Context{
		Messages: messages,
		Tools:    []llm.ToolDefinition{tool},
	}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}

	messages = append(messages, &response)
	if response.StopReason != llm.StopReasonToolUse {
		fmt.Println(response.Text())
		break
	}

	for _, call := range response.ToolCalls() {
		args, err := llm.DecodeToolCall[WeatherArgs](tool, call)
		if err != nil {
			result := llm.ToolResult(call.ID, call.Name, err.Error())
			result.IsError = true
			messages = append(messages, result)
			continue
		}

		resultText := lookupWeather(args.City)
		messages = append(messages,
			llm.ToolResult(call.ID, call.Name, resultText))
	}
}
```

`lookupWeather` is application code, not part of `llm`.

## Safety constraints

- Drive the loop from `StopReason`.
- Append the assistant message before all tool results.
- Return one result for every call, including failures.
- Set a maximum turn count.
- Inspect `response.Diagnostics` and validate again before side effects.
