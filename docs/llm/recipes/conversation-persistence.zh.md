# 保存与恢复对话

`llm` 不保存会话状态。每次调用都由应用传入 `Context`；要在进程重启后继续对话，应用需要保存并恢复这个 `Context`。

`Context` 的 JSON 包含系统指令、消息和工具定义。消息会保留其具体类型，因此恢复后仍可继续发送给模型服务。

## 适用范围

| 需求 | `llm` 提供的部分 | 应用负责的部分 |
|---|---|---|
| 在下一轮继续对话 | `Context.Messages` 和类型化消息 | 保存、读取并追加消息 |
| 进程重启后恢复 | `Context` 的 JSON 编解码 | 数据库或文件存储 |
| 保存工具调用 | `ToolCall`、`ToolResultMessage` 的 JSON | 工具执行状态和幂等记录 |
| 识别一段会话 | 当前材料中未提供会话 ID | 会话 ID、用户和租户归属 |
| 选择下一轮模型 | 每次调用显式传入 `Model` | 保存模型选择或重新选择模型 |
| 控制历史长度 | `Model.ContextWindow`、`IsContextOverflow` | 删除、压缩或摘要旧消息 |

## 运行前准备

示例使用 DeepSeek 模型，并将会话保存到当前目录的 `conversation.json`：

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## 完整程序

程序完成第一轮请求，将完整 `Context` 写入 JSON 文件，读取文件后追加新的用户消息并继续请求。

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	conversation := llm.PromptWithSystem(
		"Answer briefly.",
		"Name one Go web framework.",
	)

	first := complete(model, conversation)
	conversation.Messages = append(conversation.Messages, &first)

	stored, err := json.MarshalIndent(conversation, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("conversation.json", stored, 0o600); err != nil {
		log.Fatal(err)
	}

	raw, err := os.ReadFile("conversation.json")
	if err != nil {
		log.Fatal(err)
	}
	var restored llm.Context
	if err := json.Unmarshal(raw, &restored); err != nil {
		log.Fatal(err)
	}
	restored.Messages = append(restored.Messages,
		llm.UserText("Which year was it first released?"))

	second := complete(model, restored)
	fmt.Println(second.Text())
}

func complete(model llm.Model, input llm.Context) llm.AssistantMessage {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	response, err := llm.Complete(ctx, model, input,
		llm.StreamOptions{MaxTokens: 300})
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

首次运行会创建 `conversation.json`。文件保存了系统指令、第一条用户消息和第一轮的完整助手消息；第二次请求在这些历史之后继续。

## 保存的内容

保存完整 `Context`，不要只保存显示给用户的文本。助手消息可能包含推理内容、工具调用、模型服务和模型标识、token 用量、诊断信息及结束原因；后续调用、重放和排查可能需要这些信息。

需要逐条存储消息时，使用 `MarshalMessage` 和 `UnmarshalMessage`。它们会在 JSON 中写入角色信息，以便恢复为 `UserMessage`、`AssistantMessage` 或 `ToolResultMessage`。未知角色、未知内容类型和格式错误的 JSON 会返回 error。

`Context` 会保存 `SystemPrompt`、`Messages` 和 `Tools`；字段与消息类型统一见[消息与上下文](../conversations.md)。它不包含会话 ID、创建时间、用户或租户 ID、数据库版本，以及下一轮要使用的 `Model`。应用通常需要用自己的记录结构包裹 `Context`。

## 选择存储方式

| 方式 | 适用情况 | 行为 |
|---|---|---|
| `json.Marshal(Context)` | 一次读写整段短对话 | 同时保存系统指令、消息和工具定义 |
| `MarshalMessage` | 数据库逐条存储或 JSON Lines | 每次保存一条带角色和内容类型的消息 |
| 应用自定义记录 | 需要会话 ID、版本、租户和审计字段 | 将 `Context` 或单条消息作为记录的一部分 |

逐条存储时不能只保存 `Messages` 而忽略 `SystemPrompt` 和 `Tools`。恢复时应按照原始轮次顺序读取消息。

## 对话恢复过程

1. 应用创建 `Context`，并追加用户消息。
2. 请求结束后，将 `&AssistantMessage` 追加到 `Context.Messages`。
3. 序列化并保存完整 `Context`。
4. 读取 JSON 并反序列化为 `Context`。
5. 追加下一条用户消息，再将恢复后的 `Context` 传给 `Complete` 或 `Stream`。

`Context` 本身会保存 `SystemPrompt` 和 `Tools`。如果存储方案只保存 `Messages`，应用还必须另行保存这两个字段。

## 轮次与并发更新

- 用户消息、助手消息和工具结果必须保持原始顺序。助手发起工具调用时，先保存完整助手消息，再保存每个对应的工具结果。
- `Complete` 返回 `err != nil` 时可能同时返回部分助手消息。是否保存部分结果必须由应用明确决定。
- 多个请求同时更新同一会话时，应使用事务、版本号或乐观锁，避免后写入的请求覆盖较新的历史。
- 保存成功后再确认业务请求完成，或使用可重试的事务，避免模型已响应但历史未持久化。
- `llm` 不会锁定 `Context`。同一个消息切片不应在发送请求的同时被其他 goroutine 修改。

## 存储边界

- 历史记录可能包含提示词、工具结果、图片和模型服务返回的签名，应按敏感数据处理。
- 文件或数据库应使用加密、访问控制和最小权限；不要在日志中记录完整会话。
- 保留期限、删除规则和租户归属由应用的存储系统负责。
- 每次请求前应按 `Model.ContextWindow` 控制历史长度。`llm` 能报告上下文超限，但不会自动压缩或摘要历史。
- 追加 `&AssistantMessage`，不要只追加其文本。这样才能保留消息类型和元数据。
- 更换模型时保留原始历史，由协议适配器在发送前创建目标模型所需的副本。转换规则见[对话中更换模型](model-switching.md)。
- 当前 JSON 没有应用级 schema 版本字段。长期存储时，应用应为自己的外层记录定义版本和迁移策略。
