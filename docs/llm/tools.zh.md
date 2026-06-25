# 工具

## 类型化工具

从 Go 结构体生成与提供方兼容的 JSON Schema，而无需手写工具参数。同一个类型既用于校验、
强制转换，也用于解码模型返回的工具调用。

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

没有 `omitempty` 的字段为必填。生成的 schema 完全内联，并省略了 `$schema`、`$id`、
`$ref`、`$defs` 等文档元数据。

提供方流式传来的工具参数可能从不完整的 JSON 中恢复而来。在执行带副作用的工具前，
请先阅读[流式诊断](streaming.md#tool-call-deltas-and-diagnostics)。

## 协议特定的工具选择

工具选择保留各协议自身的原生写法。通过 `ProtocolOptions` 提供；客户端会校验它的类型
与所选模型协议是否匹配，以及被命名的工具是否存在于请求 context 中。

OpenAI 兼容的 Chat Completions 使用 `required` 和 function 选择：

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.OpenAICompletionsStreamOptions{
		ToolChoice: llm.OpenAIToolChoiceRequired,
		// 若要强制调用某一个 function：
		// ToolChoice: llm.OpenAIToolChoiceFunction{Name: "get_weather"},
	},
}
```

Anthropic Messages 使用 `any` 和 tool 选择：

```go
options := llm.StreamOptions{
	ProtocolOptions: &llm.AnthropicStreamOptions{
		ToolChoice: llm.AnthropicToolChoiceAny,
		// 若要强制调用某一个工具：
		// ToolChoice: llm.AnthropicToolChoiceTool{Name: "get_weather"},
	},
}
```

两种协议都提供 `Auto` 和 `None` 常量。任何显式的工具选择都要求 `Context.Tools`
中至少有一个工具。
