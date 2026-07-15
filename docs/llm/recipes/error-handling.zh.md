# 处理请求失败

请求失败与模型正常结束是两类不同状态。先处理返回的 `error`，再根据 `AssistantMessage.StopReason` 判断模型为何停止。

## 失败发生在哪个阶段

错误发生在三个阶段，处理方式不同：

| 阶段 | 信号 | 示例 |
|---|---|---|
| 请求开始前 | `Stream` 或 `Complete` 直接返回 `error` | 选项无效、协议适配器未注册、请求构造失败 |
| 响应过程中 | `EventError`，或 `Complete` 返回部分消息和 `error` | 鉴权、限流、HTTP 或解码失败 |
| 模型正常停止 | `error` 为 nil，通过 `StopReason` 表示原因 | 正常完成、达到 token 上限或请求工具 |

## 统一处理 Complete 结果

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
		// message 可能含部分文本和 token 用量；是否保留由调用方决定。
		return message, fmt.Errorf("complete %s/%s: %w",
			model.Provider, model.ID, err)
	}

	switch message.StopReason {
	case llm.StopReasonStop:
		return message, nil
	case llm.StopReasonToolUse:
		return message, fmt.Errorf("model requested tool execution")
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

## 何时重试

- 只有操作可安全重放时，才重试临时网络、限流或模型服务不可用错误。
- 协议适配器缺失、选项无效、工具无效和未知模型 ID 必须先修改配置，原样重试无效。
- `MaxRetries` 控制 SDK 重试；应用重试会重新执行整个逻辑请求，可能重复展示文本或产生工具副作用。
- 不执行 `EventError`、`StopReasonError` 或 `StopReasonAborted` 消息中的工具调用。
- `StopReasonLength` 可选择接受截断、提高上限，或追加部分 assistant turn 并明确要求续写。

## 上下文溢出

```go
if llm.IsContextOverflow(message, model.ContextWindow) {
	// 由应用压缩、摘要或删除旧消息，然后重试。
}
```

`llm` 能检测显式 provider error 和部分基于 token 用量的静默溢出，但不会决定删除哪些消息。

## 记录排障信息

记录提供方和模型 ID、协议、结束原因、响应 ID、尝试次数、延迟和脱敏后的诊断信息。默认不要记录 API 密钥、完整请求头、原始请求体、图片或整段历史。按症状排查见[排障](../troubleshooting.md)。
