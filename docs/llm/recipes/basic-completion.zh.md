# 基础生成

## 本场景实现什么

这个命令行程序从目录解析模型，确认协议 adapter 已注册，发送 system prompt 和 user prompt，并打印文本、停止原因、usage 与成本估算。

批处理、只返回完整响应的 HTTP handler，以及由应用控制的对话或工具循环中的单轮请求，适合使用 `Complete`。它在内部仍然消费 provider 的流式 API。

## 前置条件

```sh
go get github.com/ktsoator/or/llm@latest
export DEEPSEEK_API_KEY=your-key
```

## 完整程序

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai"
)

func ptr[T any](value T) *T { return &value }

func main() {
	model, ok := llm.LookupModel("deepseek", "deepseek-v4-flash")
	if !ok {
		log.Fatal("model is not present in the embedded catalog")
	}
	if !llm.SupportsProtocol(model.Protocol) {
		log.Fatalf("protocol %q has no registered adapter", model.Protocol)
	}

	input := llm.PromptWithSystem(
		"You are a concise Go reviewer.",
		"When should a Go service use a channel instead of a mutex?",
	)
	response, err := llm.Complete(context.Background(), model, input,
		llm.StreamOptions{
			Temperature: ptr(0.2),
			MaxTokens:   400,
		})
	if err != nil {
		log.Printf("partial response: %q", response.Text())
		log.Fatal(err)
	}

	fmt.Println(response.Text())
	fmt.Printf("\nstop=%s input=%d output=%d cost=$%.6f\n",
		response.StopReason,
		response.Usage.Input,
		response.Usage.Output,
		response.Usage.Cost.Total,
	)
}
```

运行：

```sh
go run .
```

正文由 provider 生成。正常响应通常以 `stop=stop` 结束；token 数和成本取决于 provider 返回的 usage。

## 调用流程

1. `LookupModel` 查询嵌入目录，以 `(Model, bool)` 返回，不会 panic。
2. 副作用 import 注册 OpenAI Chat Completions adapter。
3. `Complete` 校验选项并解析 `DEEPSEEK_API_KEY`。
4. Adapter 转换历史、序列化 provider 请求并读取流。
5. `Complete` 返回 `EventDone` 中的消息；若收到 `EventError`，返回部分消息和 error。

## 参数与结果

| 值 | 含义 |
|---|---|
| `Temperature` | 指针用于区分“未设置”和显式值；provider 是否接受该值各不相同 |
| `MaxTokens` | 输出上限；`0` 保留协议自己的默认行为 |
| `response.Text()` | 按顺序连接全部文本内容块 |
| `response.StopReason` | 生成结束原因；执行工具前必须先判断 |
| `response.Usage` | Provider 报告的 token 与按目录价格计算的成本估算 |

## 失败与边界

- 动态输入未知时 `GetModel` 会 panic；配置或用户输入应使用 `LookupModel`。
- 目录模型可能属于未实现协议。调用前检查 `SupportsProtocol`，或从 `GetRunnableModels` 选择。
- 缺少 key 时请求在到达 provider 前失败。对外展示配置状态时使用 `AuthStatus`。
- Error 非 nil 时仍可能带部分文本和 usage。应用需决定是否展示或持久化部分结果。
- `Usage.Cost` 不是账单。

可复用的响应策略见[错误处理](error-handling.md)，动态模型选择见[模型与鉴权发现](provider-discovery.md)。
