# 对话持久化

## 本场景实现什么

应用完成第一轮，把完整类型化 `Context` 保存为 JSON，恢复后追加下一条 user 消息，再继续对话。

`llm` 是无状态的。请求携带的历史切片就是对话；本包不创建 session，也不写数据库。

## 完整程序

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	ctx := context.Background()
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	history := []llm.Message{llm.UserText("Name one Go web framework.")}

	first, err := llm.Complete(ctx, model,
		llm.NewContext(history...), llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err)
	}
	history = append(history, &first)

	stored, err := json.MarshalIndent(llm.Context{Messages: history}, "", "  ")
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

	second, err := llm.Complete(ctx, model, restored,
		llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(second.Text())
}
```

## 必须保存什么

应保存完整消息 JSON，而不是只保存渲染文本。Assistant content 可能含 reasoning 签名、工具调用、provider/model 身份、usage、diagnostics 和停止原因，这些字段会影响后续重放和排障。

按行存储时，对单条消息使用 `MarshalMessage` 和 `UnmarshalMessage`。未知角色、未知内容类型与畸形 JSON 都会返回 error。

## 生产策略

- 序列化历史可能包含提示词、工具结果、图片和 provider 签名，应按敏感数据处理。
- 存储需加密或访问控制，使用严格文件/数据库权限，不记录整段历史。
- 保留周期、删除和租户归属由 `llm` 之外的系统定义。
- 每次请求前按 `Model.ContextWindow` 管理长度；`llm` 能检测溢出，但不会自动摘要。
- 追加 `&AssistantMessage`，不要只追加文本，以保留具体类型和 metadata。
- `SystemPrompt` 属于请求 `Context`；若要跨轮次保持，应用需单独持久化。
