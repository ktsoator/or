# 流式聊天

## 本场景实现什么

这个终端程序边到达边打印文本，保存最终组装消息，记录流错误，并在超时或取消后继续排空通道。

首 token 延迟重要，或 UI 需要区分文本、thinking 与工具进度时使用 `Stream`。只需要最终消息时使用 `Complete`。

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

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	events, err := llm.Stream(ctx, model,
		llm.Prompt("Write a three-line poem about Go concurrency."),
		llm.StreamOptions{MaxTokens: 300})
	if err != nil {
		log.Fatal(err) // 初始化失败，没有创建 stream
	}

	var final *llm.AssistantMessage
	var streamErr error
	for event := range events { // 始终读到通道关闭
		switch event.Type {
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			final = event.Message
		case llm.EventError:
			final = event.Message
			streamErr = event.Err
		}
	}

	if streamErr != nil {
		if final != nil {
			log.Printf("partial text: %q", final.Text())
		}
		log.Fatal(streamErr)
	}
	if final == nil {
		log.Fatal("stream closed without a terminal message")
	}
	fmt.Printf("\nstop=%s tokens=%d\n", final.StopReason, final.Usage.TotalTokens)
}
```

设置 `DEEPSEEK_API_KEY` 后运行：

```sh
go run .
```

## 事件生命周期

```text
EventStart
  → EventTextStart
  → EventTextDelta ...
  → EventTextEnd
  → EventDone | EventError
  → channel close
```

推理和工具调用各自有 start/delta/end 事件。非终止事件包含 `Partial`，它是当前响应快照；终止事件改用 `Message`。

## 为什么接收循环必须这样写

`Stream` 返回无缓冲通道。取消 `ctx` 会请求 adapter 和 HTTP 请求停止，但不能替代通道消费。立即退出循环可能让 producer 阻塞在发送终止事件上。应先记录错误，继续 range 到关闭，然后再返回错误。

选项无效、adapter 缺失或请求构造失败时，`Stream` 直接返回 error。启动后的 provider 或解码失败以 `EventError` 到达。

## 接入建议

| 关注点 | 推荐处理 |
|---|---|
| HTTP 流式输出 | Flush `EventTextDelta`；不要在每个事件上整体发送 `Partial` |
| 客户端断开 | 取消 context，再由内部 goroutine 继续排空 LLM 通道 |
| 工具执行 | 等待 `EventDone`；流中的参数可能仍不完整 |
| 重试 | SDK 会在初始化或请求期重试；已展示部分答案后是否重放必须由产品策略决定 |
| 背压 | 通道无缓冲，consumer 越慢，producer 也越慢 |

流没有 `Close` 或 `Abort` 方法。使用 context 取消，通过通道关闭确认结束。
