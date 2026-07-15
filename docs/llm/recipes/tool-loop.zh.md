# 工具循环

## 本场景实现什么

一个有轮次上限的循环允许模型请求类型化天气工具，校验并解码参数，执行应用代码，为每个调用追加结果，再把更新后的历史发回模型生成最终答案。

`llm` 只定义和传输工具调用，从不执行工具。鉴权、超时、幂等、审计和副作用策略都属于应用。

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

type WeatherArgs struct {
	City string `json:"city" jsonschema:"description=City name,minLength=1"`
	Days int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func main() {
	ctx := context.Background()
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	weather := llm.MustTool[WeatherArgs]("get_weather", "Get a weather forecast")
	messages := []llm.Message{
		llm.UserText("What should I pack for three days in Beijing?"),
	}

	for turn := 0; turn < 8; turn++ {
		response, err := llm.Complete(ctx, model, llm.Context{
			Messages: messages,
			Tools:    []llm.ToolDefinition{weather},
		}, llm.StreamOptions{MaxTokens: 800})
		if err != nil {
			log.Fatal(err)
		}

		messages = append(messages, &response) // assistant 必须位于 result 前
		switch response.StopReason {
		case llm.StopReasonStop:
			fmt.Println(response.Text())
			return
		case llm.StopReasonToolUse:
			// 在下方继续。
		default:
			log.Fatalf("generation stopped: %s: %s",
				response.StopReason, response.ErrorMessage)
		}

		for _, call := range response.ToolCalls() {
			args, err := llm.DecodeToolCall[WeatherArgs](weather, call)
			if err != nil {
				result := llm.ToolResult(call.ID, call.Name, err.Error())
				result.IsError = true
				messages = append(messages, result)
				continue
			}

			// 实际应用在这里调用经过授权的工具实现。
			text := fmt.Sprintf("%s: sunny, 25C, for %d days", args.City, args.Days)
			messages = append(messages, llm.ToolResult(call.ID, call.Name, text))
		}
	}
	log.Fatal("tool loop exceeded 8 turns")
}
```

## 循环契约

1. 定义 `ToolDefinition`；静态启动声明可使用 `MustTool`，定义无效时会 panic。
2. 将工具和当前消息一起发送。
3. 先追加 assistant 消息，再追加工具结果。
4. 只有 `StopReasonToolUse` 才继续执行工具。
5. 每个 `ToolCall` 必须对应一个 `ToolResultMessage`。
6. 反复发送扩展后的历史，直到正常停止或达到应用上限。

`DecodeToolCall` 会按生成的 schema 校验并解码为 `WeatherArgs`。它可以转换数字字符串等常见错误，但参数转换不等于授权。

## 失败与安全策略

- 有副作用前检查 `response.Diagnostics`；通常应拒绝以 `partial` 或 `invalid` 恢复的参数。
- 解码或业务错误用 `IsError=true` 回传给模型，不要因为可纠正调用让整个循环崩溃。
- 为每个工具设置 deadline 和取消；示例只用同步固定结果来突出 LLM 流程。
- Schema 之外仍要做 allowlist 和用户鉴权。合法参数也可能请求被禁止的操作。
- 写操作使用幂等键，避免 provider 或应用重试重复产生外部副作用。
- 限制轮次、并发调用、payload 和总成本。

协议特定的强制工具选择见[工具](../tools.md#协议特定的工具选择)。
