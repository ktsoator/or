# 单次文本生成

`Complete` 等待模型请求结束，并返回一个完整的 `AssistantMessage`。适用于批处理、只需返回完整结果的服务接口，以及对话或工具循环中的单轮请求。

`Complete` 会在内部读取并汇总流式响应，调用方只获得请求结束后的消息。需要在生成过程中展示文本、推理内容或工具调用进度时，使用[流式响应](streaming-chat.md)。

## 适用范围

| 场景 | 是否适合使用 `Complete` |
|---|---|
| 后台批量生成、摘要或分类 | 适合；调用方只需要最终结果 |
| 服务接口等待模型完成后一次性返回 | 适合；可直接处理 `AssistantMessage` |
| 多轮对话中的单轮请求 | 适合；历史记录仍由应用维护 |
| 工具调用循环中的一次模型请求 | 适合；收到结果后由应用判断是否执行工具 |
| 页面逐字显示生成内容 | 不适合；应使用 `Stream` |
| 需要实时展示推理或工具参数 | 不适合；应消费流式事件 |

## 构造请求内容

`Complete` 的第三个参数是 `Context`。纯文本请求可使用以下辅助函数：

| 构造方式 | 生成的内容 |
|---|---|
| `llm.Prompt(text)` | 一条包含文本的用户消息 |
| `llm.PromptWithSystem(system, user)` | 系统指令和一条用户消息 |
| `llm.NewContext(messages...)` | 由已有的类型化消息构造上下文 |
| `llm.Context{...}` | 同时设置系统指令、消息和工具定义 |

例如，只发送用户问题时可以写成：

```text
input := llm.Prompt("Summarize Go context cancellation in three sentences.")
```

系统指令属于 `Context.SystemPrompt`，不会作为普通历史消息追加。对话历史、图片和工具需要使用完整 `Context`；对应示例见[保存与恢复对话](conversation-persistence.md)、[发送图片](vision.md)和[执行工具调用](tool-loop.md)。

## 运行前准备

示例从内置模型清单中查找模型，检查当前程序是否已注册所需的协议适配器，随后发送系统指令和用户消息，最后输出文本、结束原因、token 用量和成本估算。

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

func ptr[T any](value T) *T { return &value }

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok {
		log.Fatal("model is not present in the embedded catalog")
	}
	if !llm.SupportsProtocol(model.Protocol) {
		log.Fatalf("protocol %q has no registered adapter", model.Protocol)
	}

	input := llm.PromptWithSystem(
		"You are a concise Go reviewer.",
		"When should a Go service use a channel instead of a mutex?",
	)
	response, err := llm.Complete(ctx, model, input,
		llm.StreamOptions{
			Temperature: ptr(0.2),
			MaxTokens:   400,
		})
	if err != nil {
		log.Printf("partial response: %q", response.Text())
		log.Fatal(err)
	}

	fmt.Println(response.Text())
	fmt.Printf("\nstop=%s input=%d output=%d cost=$%.6f\n",
		response.StopReason,
		response.Usage.Input,
		response.Usage.Output,
		response.Usage.Cost.Total,
	)
}
```

运行：

```sh
go run .
```

输出文本由模型服务生成。正常结束时，程序通常会输出 `stop=stop`。token 用量来自模型服务返回的数据；成本按内置模型清单中的价格估算。

## 请求过程

1. `LookupModel` 按提供方和模型 ID 从内置模型清单中查找模型，返回 `(Model, bool)`。
2. 空导入 `llm/openai` 会在包初始化时注册 OpenAI Chat Completions 协议适配器。
3. `Complete` 校验 `StreamOptions`，并从提供方配置或环境变量中读取 API 密钥。
4. 协议适配器转换消息、序列化请求，并读取模型服务的响应流。
5. 收到 `EventDone` 时，`Complete` 返回最终消息；收到 `EventError` 时，可能同时返回已生成的部分消息和错误。

## 示例中的请求设置

程序用 `context.WithTimeout` 限制整个调用最多运行 45 秒，并显式设置 `Temperature` 和 `MaxTokens`。`Temperature` 使用指针区分“未设置”和显式数值；`MaxTokens` 控制输出上限。

`StreamOptions.Timeout` 限制单次 HTTP 尝试，不替代整个调用的 context。重试、凭证、请求头、推理等级和 Hook 等其他字段统一见[请求选项](../configuration.md)。

## 读取返回结果

示例读取 `Text()`、`StopReason` 和 `Usage`。服务代码还应保留完整 `AssistantMessage`，以便后续读取内容块、响应 ID、诊断或部分结果。全部字段和停止原因统一见[响应与用量](../results.md)。

## 常见失败与处理原则

- 提供方或模型 ID 不存在时，`GetModel` 会 panic。配置或用户输入应使用 `LookupModel`。
- 清单中收录的模型可能属于未实现协议。调用前检查 `SupportsProtocol`，或从 `GetRunnableModels` 选择。
- 缺少 API 密钥时，请求会在发送到模型服务前失败。对外展示配置状态时使用 `AuthStatus`。
- `err != nil` 时，返回消息仍可能包含部分文本和 token 用量。应用应明确决定是否展示或持久化这些部分结果。
- `Usage.Cost` 是估算值，不是账单。

## 在服务中使用

- 模型 ID 来自配置或用户输入时，在启动或请求入口使用 `LookupModel` 校验。
- 为每次调用设置 context 截止时间；`StreamOptions.Timeout` 只限制单次 HTTP 尝试。
- 根据 `StopReason` 决定返回文本、执行工具、提示截断或报告失败。
- 如果 `err != nil`，先决定是否保留部分消息，再返回应用自己的错误类型。
- 日志可记录提供方、模型、响应 ID、结束原因、token 用量和延迟，不要记录 API 密钥或完整提示词。

统一的响应策略见[处理请求失败](error-handling.md)。动态选择模型和检查凭证见[查找模型与检查凭证](provider-discovery.md)。
