# 工具

## 类型化工具

从 Go 结构体生成与提供方兼容的 JSON Schema，而无需手写工具参数。同一个类型既用于校验、
强制转换，也用于解码模型返回的工具调用。

**1. 用结构体描述参数。** `jsonschema` 标签会转化为 schema 约束。没有 `omitempty`
的字段为必填。生成的 schema 完全内联，并省略了 `$schema`、`$id`、`$ref`、`$defs`
等文档元数据。

```go
type WeatherArgs struct {
	City  string `json:"city" jsonschema:"description=City name,minLength=1"`
	Units string `json:"units,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
	Days  int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}
```

**2. 从类型构建工具**,并挂到请求 context 上。

```go
weatherTool := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast")

input := llm.Context{
	Messages: []llm.Message{
		llm.UserText("What's the weather in Shanghai for the next 3 days?"),
	},
	Tools: []llm.ToolDefinition{weatherTool},
}
```

**3. 发送请求并读回工具调用。** `response.ToolCalls()` 返回模型发起的每个调用;先把
助手消息追加进历史,后续的工具结果才能跟在它后面。

```go
response, err := llm.Complete(ctx, model, input, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}
messages = append(messages, &response)
```

**4. 解码每个调用、返回结果,再次询问。** `DecodeToolCall` 会按 schema 校验参数并解码
进 `WeatherArgs`,得到可直接使用的值。

```go
for _, toolCall := range response.ToolCalls() {
	arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, toolCall)
	if err != nil {
		log.Fatal(err)
	}
	result := fmt.Sprintf("%s will be sunny for %d days.", arguments.City, arguments.Days)
	messages = append(messages, llm.ToolResult(toolCall.ID, toolCall.Name, result))
}
```

把工具结果放进第二次 `Complete` 发回去,模型就能据此给出最终答案。

<details>
<summary>完整程序</summary>

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // 注册 OpenAI 兼容协议
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

</details>

当类型无法生成有效 schema 时，`MustTool` 会 panic，适合在启动阶段声明的工具。若工具
是动态构建的、需要处理失败而非崩溃，请改用返回 error 的 `NewTool`。

## 运行工具循环

上面的示例为清晰起见只处理了一轮。真实应用需要循环:模型可能调用工具、读取结果，
再调用更多工具，最后才作答。`StopReason` 会告诉你当前处于哪种情况，因此应当依据它
来控制循环，而不是仅凭是否存在工具调用。

- `StopReasonToolUse` — 模型需要工具结果。执行这些调用，逐个追加结果，再次调用模型。
- `StopReasonStop` — 模型已作答。返回 `response.Text()`。
- `StopReasonLength` — 输出触达 token 上限，本轮被截断。
- `StopReasonError` / `StopReasonAborted` — 请求失败或被取消。绝不要执行这类响应中的
  工具调用。

```go
for {
	response, err := llm.Complete(ctx, model, llm.Context{
		Messages: messages,
		Tools:    []llm.ToolDefinition{weatherTool},
	}, llm.StreamOptions{})
	if err != nil {
		log.Fatal(err) // 失败的响应仍可能携带部分内容
	}

	if response.StopReason != llm.StopReasonToolUse {
		fmt.Println(response.Text())
		break
	}

	// 在历史中，助手消息必须排在它的工具结果之前。
	messages = append(messages, &response)
	for _, toolCall := range response.ToolCalls() {
		arguments, err := llm.DecodeToolCall[WeatherArgs](weatherTool, toolCall)
		if err != nil {
			// 将错误返回给模型，让它纠正这次调用。
			result := llm.ToolResult(toolCall.ID, toolCall.Name, err.Error())
			result.IsError = true
			messages = append(messages, result)
			continue
		}
		messages = append(messages, llm.ToolResult(
			toolCall.ID, toolCall.Name, runWeather(arguments)))
	}
}
```

用最大轮数为循环设置上界，避免行为异常的模型无限循环。可运行的
[`tool` 示例](https://github.com/ktsoator/or/tree/main/example/llm/tool)
展示了带诊断和错误处理的完整循环。

## 执行前校验

`DecodeToolCall` 会按工具 schema 校验参数并一步解码进你的结构体，这是大多数应用采用的
路径。当参数没有对应的 Go 类型时，可改为校验成通用 map:

- `ValidateToolCall(tools, call)` — 按名称匹配工具，然后校验并强制转换;以
  `map[string]any` 返回参数。
- `ValidateToolArguments(tool, call)` — 针对一个已知工具进行校验。
- `ParseToolArguments(raw)` — 对原始参数 JSON 做尽力解析，不做 schema 校验;搭配
  `ParseToolArgumentsMode` 可得知 JSON 是严格、已修复、部分还是无效。

提供方流式传来的工具参数可能从不完整的 JSON 中恢复而来。稳妥的应用会拒绝 `partial`
和 `invalid` 的参数，并返回一个工具错误让模型重试。在执行带副作用的工具前，请先阅读
[流式诊断](streaming.md#工具调用增量与诊断)。

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
