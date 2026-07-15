# 对话中更换模型

示例先使用 DeepSeek 起草，再将同一段历史发送给 MiniMax 复核。两个模型使用不同的请求与响应协议。

应用保存与模型服务无关的 `Message`。下一轮更换模型或协议时，无需手动转换历史。

## 适用场景

| 场景 | 切换方式 |
|---|---|
| 一个模型起草，另一个模型复核 | 保存第一轮完整响应，再向复核模型追加明确任务 |
| 默认模型不可用时回退 | 保留原始 `Context`，使用备用模型重新请求 |
| 按成本或延迟分层 | 每轮根据应用策略选择模型 |
| 图片模型完成识别后转文本模型 | 图片会按目标能力转换；必要信息应先形成文字 |
| 工具循环中途更换模型 | 保留完整工具调用和结果，并重新提供工具定义 |

模型选择不存储在 `Context` 中。应用必须在每次 `Complete` 或 `Stream` 调用时传入目标 `Model`。

## 运行前准备

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=...
export MINIMAX_CN_API_KEY=...
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
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	draft := llm.GetModel("deepseek", "deepseek-v4-flash")
	review := llm.GetModel("minimax-cn", "MiniMax-M2.7")

	conversation := llm.PromptWithSystem(
		"Produce concise, operationally safe answers.",
		"Draft a database migration checklist.",
	)
	first := complete(ctx, draft, conversation)
	fmt.Printf("[%s] %s\n", draft.Provider, first.Text())

	conversation.Messages = append(conversation.Messages, &first)
	conversation.Messages = append(conversation.Messages,
		llm.UserText("Review the checklist for missing rollback steps."))
	second := complete(ctx, review, conversation)
	fmt.Printf("[%s] %s\n", review.Provider, second.Text())
}

func complete(ctx context.Context, model llm.Model,
	input llm.Context) llm.AssistantMessage {
	requestCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	response, err := llm.Complete(requestCtx, model, input,
		llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		log.Fatal(err)
	}
	return response
}
```

运行：

```sh
go run .
```

程序设置两分钟总时限，并为每次模型调用设置 45 秒时限。第二轮继续使用同一个 `Context`，因此系统指令和完整消息历史都会保留。

## 切换前检查

| 检查项 | 相关接口或字段 |
|---|---|
| 目标模型是否存在 | `LookupModel` |
| 当前程序是否已注册目标协议 | `SupportsProtocol` 或 `GetRunnableModels` |
| 目标提供方凭证是否已配置 | `AuthStatus` |
| 文本、图片和推理能力 | `Model.Input`、`Model.Reasoning` |
| 上下文窗口与输出上限 | `Model.ContextWindow`、`Model.MaxTokens` |

通过这些检查只表示本地配置可路由，不保证凭证有效或线上模型一定接受请求。上线前仍需对目标模型执行实际请求测试。

## 历史转换

协议适配器会在序列化前自动调用 `TransformMessages`。应用继续传入原始 `Context.Messages` 即可，不要先手动改写或覆盖历史。图片降级、推理签名、失败消息和工具结果的完整转换规则统一见[消息与上下文](../conversations.md#历史与模型转换)。

转换只作用于本次发送的副本。因此切回原模型时，应用仍可使用原始图片、签名和工具调用。显式调用 `TransformMessages` 主要用于测试转换结果或实现自定义协议适配器。

## 系统指令、工具与结果

- `SystemPrompt` 和 `Tools` 属于 `Context`，不属于 `Messages`。切换时应传递完整 `Context`，或明确重新设置这些字段。
- 工具定义不会从旧的助手消息中恢复。目标模型需要继续调用工具时，必须在新请求的 `Context.Tools` 中再次提供定义。
- 每个助手工具调用都必须有匹配的工具结果。缺失结果会被转换为合成错误结果，以保持协议历史有效。
- 历史中的 `Usage`、`ResponseID` 和模型标识仍表示原始响应，不会改写成目标模型的数据。
- 新模型的回答应作为新的 `AssistantMessage` 追加，不要覆盖前一个模型的原始消息。

## 能力与语义差异

消息能够转换，不代表两个模型的行为相同。目标模型可能具有更小的上下文窗口、不支持图片或推理、使用不同工具选择策略，或者对系统指令有不同解释。

切换提示应明确新模型的任务，例如“复核前一答案”或“在不改动结论的前提下压缩”。不要假设新模型知道切换原因，也不要把前一模型的隐藏推理当作共享上下文。

## 使用边界

- 必须导入两个协议包并配置两个凭证。
- 更换模型服务不会迁移其缓存或服务端会话状态。
- 上下文窗口仍由应用管理。重试前使用 `IsContextOverflow` 并压缩旧历史。
- 应保存完整 assistant 消息，包括签名和工具调用；只保存 `Text()` 会丢失重放 metadata。
- 即使消息传输兼容，不同模型的输出语义也可能不同，应评估所选组合的交接提示。
- 切换失败时保留原始历史和当前模型选择，避免把未完成的目标模型响应写入正式会话。
- 不同模型的价格和 token 计算可能不同。分别记录每个 `AssistantMessage.Usage`，不要用目标模型价格重新计算旧响应。
- 回退请求可能重复产生文本或工具调用。涉及副作用时必须使用幂等键，并明确哪些轮次允许重试。

完整类型历史的 JSON 存储见[保存与恢复对话](conversation-persistence.md)。
