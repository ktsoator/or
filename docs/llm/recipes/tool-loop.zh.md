# 执行工具调用

模型请求工具后，应用校验并解码参数，执行自己的工具实现，将结果追加到历史，再继续请求模型。示例使用天气工具，并限制最多八轮。

`llm` 只定义和传输工具调用，不会执行工具。鉴权、超时、幂等、审计和副作用策略由应用负责。

## 责任边界

| 环节 | `llm` 提供 | 应用负责 |
|---|---|---|
| 工具声明 | 从 Go 结构体生成 JSON Schema | 设计工具名称、说明和参数约束 |
| 工具请求 | 解析 `ToolCall` 并保留调用 ID | 根据名称分派到允许的实现 |
| 参数处理 | 按 schema 转换、校验和解码 | 鉴权、业务规则和资源范围检查 |
| 工具执行 | 当前材料中未提供执行器 | 超时、取消、并发、审计和副作用 |
| 结果回传 | 构造 `ToolResultMessage` | 脱敏结果并决定错误是否可重试 |
| 循环控制 | 提供结束原因和类型化历史 | 轮次、token、成本和总时长上限 |

## 运行前准备

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

type WeatherArgs struct {
	City string `json:"city" jsonschema:"description=City name,minLength=1"`
	Days int    `json:"days" jsonschema:"minimum=1,maximum=10"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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

		calls := response.ToolCalls()
		if len(calls) == 0 {
			log.Fatal("model stopped for tool use without a tool call")
		}
		for _, call := range calls {
			if argumentsRecovered(response.Diagnostics, call.ID) {
				result := llm.ToolResult(call.ID, call.Name,
					"tool arguments were incomplete; call the tool again")
				result.IsError = true
				messages = append(messages, result)
				continue
			}
			args, err := llm.DecodeToolCall[WeatherArgs](weather, call)
			if err != nil {
				log.Printf("invalid tool call %s: %v", call.ID, err)
				result := llm.ToolResult(call.ID, call.Name,
					"invalid tool arguments; correct them and try again")
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

func argumentsRecovered(diagnostics []llm.Diagnostic, callID string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Type != llm.DiagnosticToolArgumentsRecovered {
			continue
		}
		id, _ := diagnostic.Details["toolCallId"].(string)
		if id == callID {
			return true
		}
	}
	return false
}
```

示例中的天气结果是固定字符串。实际实现应使用独立的工具超时，并在调用外部系统前完成用户授权和资源范围检查。

## 定义工具

示例使用 `MustTool` 声明启动时固定的工具，并使用 `DecodeToolCall` 校验和解码返回参数。动态构建工具时改用返回 `error` 的 `NewTool`。全部构造、Schema 标签和通用校验 API 见[工具定义与调用](../tools.md)。

Schema 校验只能判断参数形状，不能判断用户是否有权访问某个城市、文件或账户。

## 工具调用顺序

1. 定义 `ToolDefinition`。静态启动声明可使用 `MustTool`；定义无效时会 panic。
2. 将工具和当前消息一起发送。
3. 先追加 assistant 消息，再追加工具结果。
4. 只有 `StopReasonToolUse` 才继续执行工具。
5. 每个 `ToolCall` 必须对应一个 `ToolResultMessage`。
6. 反复发送扩展后的历史，直到正常停止或达到应用上限。

`DecodeToolCall` 按生成的 schema 校验并解码为 `WeatherArgs`。它可以转换数字字符串等常见格式差异，但参数能被转换不代表操作已获授权。

## 多个工具与结果回传

一轮响应可能包含多个 `ToolCall`。应用必须遍历 `response.ToolCalls()`，并为每个调用追加一个具有相同调用 ID 的结果：

- 成功时使用 `ToolResult(call.ID, call.Name, text)`。
- 参数或业务错误时仍返回 `ToolResultMessage`，并设置 `IsError = true`，让模型有机会修正调用。
- 不要把内部堆栈、数据库错误或凭证写入工具错误文本；记录详细错误时使用应用日志。
- 独立、只读的调用可以并发执行，但必须限制并发数，并将每个结果关联回正确的调用 ID。
- 有顺序依赖或副作用的调用应按应用规则串行执行，不能假设模型返回顺序就是安全执行顺序。

工具结果可以包含文字或图片。图片结果需要手动构造 `ToolResultMessage.Content`；`ToolResult` 辅助函数只创建文字结果。

## 流式工具调用

使用 `Stream` 时，工具参数通过 `EventToolCallDelta` 增量到达。增量中的 JSON 可能不完整；即使收到 `EventToolCallEnd`，参数也可能是尽力恢复的结果。

等待 `EventDone`，再从最终 `AssistantMessage.ToolCalls()` 读取调用，并检查 `Diagnostics`。`DiagnosticToolArgumentsRecovered` 表示参数没有按严格 JSON 完整解析；涉及副作用时通常应拒绝自动执行。

## 失败和安全边界

- 执行有副作用的操作前检查 `response.Diagnostics`；通常应拒绝以 `partial` 或 `invalid` 恢复的参数。
- 解码或业务错误用 `IsError=true` 回传给模型，不要因为可纠正调用让整个循环崩溃。
- 为每个工具设置 deadline 和取消；示例只用同步固定结果来突出 LLM 流程。
- schema 之外仍要做允许列表和用户鉴权。合法参数也可能请求被禁止的操作。
- 写操作使用幂等键，避免模型服务或应用重试重复产生外部副作用。
- 限制轮次、并发调用、payload 和总成本。
- 将工具名视为不可信输入，只分派到应用明确注册的实现。模型不能通过名称选择任意函数或命令。
- 工具结果也属于对话历史，可能被后续模型请求读取。回传前删除凭证、内部路径和不需要的个人信息。
- context 取消后停止启动新工具，并把取消传递给正在运行的工具实现。

协议特定的强制工具选择见[工具](../tools.md#协议特定的工具选择)。
