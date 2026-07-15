# 基础生成

## 用途

发送一条文本提示并取得完整 assistant 消息。

## 前置条件

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

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
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok || !llm.SupportsProtocol(model.Protocol) {
		log.Fatal("model is not runnable")
	}

	response, err := llm.Complete(
		context.Background(), model,
		llm.Prompt("Explain a goroutine in one sentence."),
		llm.StreamOptions{MaxTokens: 256},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
	fmt.Printf("stop=%s tokens=%d cost=$%.6f\n",
		response.StopReason,
		response.Usage.TotalTokens,
		response.Usage.Cost.Total)
}
```

## 行为

- `LookupModel` 不会在未知 ID 时 panic。
- `SupportsProtocol` 确认当前进程已导入 adapter。
- `Complete` 在底层消费流式响应。
- error 非 nil 时，返回的消息仍可能含部分内容。
