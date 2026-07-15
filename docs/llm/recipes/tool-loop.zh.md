# 工具循环

## 用途

让模型请求结构化工具，应用执行工具并把结果返回给模型。

## 程序骨架

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

`lookupWeather` 是应用函数，不属于 `llm`。

## 安全约束

- 以 `StopReason` 控制循环。
- 先追加 assistant 消息，再追加全部工具结果。
- 为每个调用返回一个结果，包括失败结果。
- 设置最大轮数。
- 有副作用前检查 `response.Diagnostics` 并再次校验参数。
