# 流式响应

`Stream` 返回事件通道。应用可以在生成过程中处理文本、推理内容和工具调用的增量，并在结束时取得完整的 `AssistantMessage`。

界面需要立即显示输出，或需要根据事件类型分别处理内容时使用 `Stream`。只需要请求结束后的完整消息时，使用[单次文本生成](basic-completion.md)中的 `Complete`。

## 适用范围

| 场景 | 使用方式 |
|---|---|
| 终端、网页或桌面界面实时显示文本 | 处理 `EventTextDelta` |
| 将推理内容与最终答案分开显示 | 分别处理 thinking 和 text 事件 |
| 展示工具调用准备进度 | 读取 tool-call 事件，但等待终止事件后再执行 |
| 记录首个内容到达时间 | 在第一个 delta 事件到达时记录 |
| 只关心最终消息 | 使用 `Complete`，无需自行消费事件 |

`Stream` 不负责把事件写入 SSE、WebSocket 或终端。应用选择输出协议，并处理客户端断开、刷新频率和部分内容是否可见。

## 运行前准备

示例使用 DeepSeek 模型。准备依赖和 API 密钥：

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## 完整程序

程序在收到文本增量时立即打印；即使发生错误或 context 被取消，也会读取事件直到通道关闭。

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
		log.Fatal(err) // 请求尚未开始，未返回事件通道
	}

	var final *llm.AssistantMessage
	var streamErr error
	for event := range events { // 持续读取，直到通道关闭
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

运行：

```sh
go run .
```

文本会随 `EventTextDelta` 到达而输出。正常结束时，程序会输出结束原因和 token 总数。

## 事件顺序

内置协议适配器会先发送 `EventStart`，再发送内容事件，最后发送一个 `EventDone` 或 `EventError`，随后关闭通道。单个文本块通常按以下顺序发送：

```text
EventStart
  → EventTextStart
  → EventTextDelta ...
  → EventTextEnd
  → EventDone | EventError
  → channel close
```

推理内容和工具调用也各自使用 start、delta、end 事件。非终止事件中的 `Partial` 是当前已组装响应的快照；`EventDone` 和 `EventError` 则通过 `Message` 提供最终或部分消息。

一条响应可以包含多个内容块。`ContentIndex` 表示事件对应 `AssistantMessage.Content` 中的哪个块。不同类型的块可能按模型服务返回的顺序交错，应用不应假设整条响应只有一个文本块。

## 示例处理的事件

上面的程序只处理三类事件：`EventTextDelta` 输出本次新增文本，`EventDone` 保存最终消息，`EventError` 保存部分消息和错误。推理界面或工具调用进度需要增加对应分支，但通道消费和终止处理不变。

全部事件类型、字段有效范围、`ContentIndex` 和 `Partial` 的语义统一见[流式事件](../streaming.md#事件参考)。本页不再维护另一份事件字段表。

## 持续读取事件

`Stream` 返回无缓冲通道。取消 `ctx` 会请求模型服务停止，但不会替代对通道的读取。若立即退出循环，发送终止事件的一方可能阻塞。

收到错误后先保存 `event.Err`，继续读取到通道关闭，再处理错误。`Stream` 在选项无效、协议适配器未注册或请求无法创建时直接返回 error；请求开始后的模型服务或解码错误通过 `EventError` 发送。

这两种错误路径需要分开处理：

| 错误位置 | `Stream` 返回值 | 事件通道 |
|---|---|---|
| 请求开始前失败 | `events == nil`，`err != nil` | 不会创建 |
| 请求开始后失败 | 初始 `err == nil` | 收到 `EventError` 后关闭 |

取消 context 后仍要继续读取通道。终止事件中的 `StopReason` 会变为 `StopReasonAborted`，`Err` 则是 context 的取消或超时错误。

## 应用侧处理

| 场景 | 处理方式 |
|---|---|
| 持续向客户端输出 | 收到 `EventTextDelta` 后写出并刷新；不要在每个事件中重复发送整个 `Partial` |
| 客户端断开 | 取消 context，并由应用继续读取事件直到通道关闭 |
| 工具调用 | 等待 `EventDone` 后再执行；流中的工具参数可能尚未完整 |
| 重试 | 已向用户显示部分内容后，是否重新请求由应用的交互策略决定 |
| 消费速度较慢 | 通道无缓冲；事件处理变慢会直接减慢响应读取 |
| 多个内容块 | 使用 `ContentIndex` 关联界面中的对应区域，不要把所有 delta 默认拼到同一字符串 |

流没有 `Close` 或 `Abort` 方法。通过 context 取消请求，以通道关闭确认处理结束。

## 生产使用检查

- 为整个调用设置 context 截止时间，并在客户端断开时取消 context。
- 只在 delta 事件中发送新增内容，避免反复传输完整 `Partial`。
- 将较慢的日志、数据库写入和遥测操作移出事件消费循环。
- 在 `EventDone` 后检查 `Message.StopReason`；工具调用和输出截断仍需要应用处理。
- 在 `EventError` 后决定是否保留已经展示的部分内容，不要自动重放导致重复输出。
- 不记录原始推理内容、工具参数、图片或完整消息，除非应用已明确完成脱敏和访问控制。
