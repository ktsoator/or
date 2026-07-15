# 模型切换

## 本场景实现什么

同一段对话先通过 OpenAI Chat Completions 调用 DeepSeek 起草，再把原有历史通过 Anthropic Messages 发给 MiniMax 复核。

应用保存 provider-neutral `Message`，下一轮更换模型或协议时不需要手动转换历史。

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	ctx := context.Background()
	draft := llm.GetModel("deepseek", "deepseek-v4-flash")
	review := llm.GetModel("minimax-cn", "MiniMax-M2.7")

	history := []llm.Message{
		llm.UserText("Draft a database migration checklist."),
	}
	first := complete(ctx, draft, history)
	fmt.Printf("[%s] %s\n", draft.Provider, first.Text())

	history = append(history, &first)
	history = append(history,
		llm.UserText("Review the checklist for missing rollback steps."))
	second := complete(ctx, review, history)
	fmt.Printf("[%s] %s\n", review.Provider, second.Text())
}

func complete(ctx context.Context, model llm.Model,
	history []llm.Message) llm.AssistantMessage {
	response, err := llm.Complete(ctx, model,
		llm.NewContext(history...), llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		log.Fatal(err)
	}
	return response
}
```

配置两个凭证：

```sh
export DEEPSEEK_API_KEY=...
export MINIMAX_CN_API_KEY=...
go run .
```

## 转换行为

每个 adapter 在序列化前调用 `TransformMessages`。它创建目标专用副本，不修改已存历史。

| 已存内容 | 目标相关行为 |
|---|---|
| 图片发送给纯文本模型 | 替换为文本占位符 |
| 同一模型产生的 reasoning | 保留兼容 thinking 和签名 |
| 其他模型产生的 reasoning | 删除 provider 私有推理内容 |
| Tool-call ID | 按目标 provider 规范化，并同步更新 result |
| 失败或取消的 assistant turn | 重放时删除 |
| 没有结果的工具调用 | 插入合成错误结果 |

## 设计约束

- 必须导入两个协议包并配置两个凭证。
- 更换 provider 不会迁移 provider 侧缓存或服务端会话状态。
- 上下文窗口仍由应用管理。重试前使用 `IsContextOverflow` 并压缩旧历史。
- 应保存完整 assistant 消息，包括签名和工具调用；只保存 `Text()` 会丢失重放 metadata。
- 即使消息传输兼容，不同模型的输出语义也可能不同，应评估所选组合的交接提示。

完整类型历史的 JSON 存储见[对话持久化](conversation-persistence.md)。
