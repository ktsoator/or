# 模型切换

## 用途

让不同模型处理同一段历史，例如一个模型起草、另一个模型复核。

## 核心代码

```go
import (
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/anthropic"
	_ "github.com/ktsoator/or/llm/openai"
)

draft := llm.GetModel("deepseek", "deepseek-v4-flash")
review := llm.GetModel("anthropic", "claude-sonnet-4-6")

messages := []llm.Message{
	llm.UserText("Draft a migration checklist."),
}

first, err := llm.Complete(ctx, draft,
	llm.Context{Messages: messages}, llm.StreamOptions{})
if err != nil {
	log.Fatal(err)
}

messages = append(messages, &first)
messages = append(messages, llm.UserText("Review the checklist for missing rollback steps."))

second, err := llm.Complete(ctx, review,
	llm.Context{Messages: messages}, llm.StreamOptions{})
```

需要同时配置两个 provider 的凭证。

## 转换行为

- reasoning 和签名不会跨模型发送；
- 图片会为纯文本目标降级；
- tool-call ID 会按目标 provider 规范化；
- 未回答工具调用会收到合成错误结果；
- 原始 `messages` 不会被永久改写。
