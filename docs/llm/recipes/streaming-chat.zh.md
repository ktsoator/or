# 流式聊天

## 用途

在文本和推理到达时立即渲染，并在终止事件中读取最终消息。

## 程序

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	events, err := llm.Stream(context.Background(), model,
		llm.Prompt("Write a three-line poem."), llm.StreamOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for event := range events {
		switch event.Type {
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			fmt.Printf("\n%d tokens\n", event.Message.Usage.TotalTokens)
		case llm.EventError:
			log.Printf("stream failed: %v", event.Err)
		}
	}
}
```

## 生命周期约束

- 持续读取事件通道直到关闭。
- 业务不再需要增量时仍应 drain 通道。
- 收到 `EventError` 后不要执行该消息中的工具调用。
- 取消使用 `context.WithCancel`；取消后仍继续读取终止事件。
