# 错误处理

## 错误层次

错误发生在三个阶段，处理方式不同：

| 阶段 | 信号 | 示例 |
|---|---|---|
| 本地初始化 | 创建 stream 前，`Stream`/`Complete` 直接返回 error | 选项无效、adapter 缺失、请求构造失败 |
| 运行时流 | `EventError`，或 `Complete` 返回部分消息和 error | 鉴权、限流、HTTP、解码失败 |
| 正常生成停止 | Error 为 nil，但 `StopReason` 不是普通 stop | Token 上限或工具请求 |

## 使用可复用策略的完整程序

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
	message, err := complete(context.Background(), model,
		llm.Prompt("Explain context cancellation briefly."))
	if err != nil {
		log.Printf("partial text: %q", message.Text())
		log.Fatal(err)
	}
	fmt.Println(message.Text())
}

func complete(ctx context.Context, model llm.Model,
	input llm.Context) (llm.AssistantMessage, error) {
	message, err := llm.Complete(ctx, model, input,
		llm.StreamOptions{MaxTokens: 800})
	if err != nil {
		// message 可能含部分文本和 usage；是否保留由调用方决定。
		return message, fmt.Errorf("complete %s/%s: %w",
			model.Provider, model.ID, err)
	}

	switch message.StopReason {
	case llm.StopReasonStop, llm.StopReasonToolUse:
		return message, nil
	case llm.StopReasonLength:
		return message, fmt.Errorf("output truncated at max token limit")
	case llm.StopReasonAborted:
		return message, context.Canceled
	default:
		return message, fmt.Errorf("generation stopped: %s: %s",
			message.StopReason, message.ErrorMessage)
	}
}
```

## 重试决策

- 只有操作可安全重放时，才重试临时网络、限流或 provider 可用性错误。
- Adapter 缺失、选项无效、工具无效和未知模型 ID 必须先修改配置，原样重试无效。
- `MaxRetries` 控制 SDK retry；应用 retry 会重放整个逻辑请求，可能重复展示文本或工具副作用。
- 不执行 `EventError`、`StopReasonError` 或 `StopReasonAborted` 消息中的工具调用。
- `StopReasonLength` 可选择接受截断、提高上限，或追加部分 assistant turn 并明确要求续写。

## 上下文溢出

```go
if llm.IsContextOverflow(message, model.ContextWindow) {
	// 由应用压缩、摘要或删除旧消息，然后重试。
}
```

`llm` 能检测显式 provider error 和部分基于 usage 的静默溢出，但不会决定删除哪些消息。

## 排障数据

记录 provider/model ID、protocol、stop reason、response ID、attempt 次数、延迟和脱敏 diagnostics。默认不要记录 API key、完整 headers、原始请求 body、图片或整段历史。按症状排查见[排障](../troubleshooting.md)。
